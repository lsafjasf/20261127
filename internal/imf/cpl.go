package imf

import (
	"os"
	"strings"
)

func parseCPLFile(path, packageLabel string) (*CPL, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	root, err := ParseNode(f)
	if err != nil {
		return nil, err
	}
	cpl := &CPL{SourceFile: path, PackageLabel: packageLabel}
	cpl.ID = NormalizeID(root.ChildText("Id"))
	cpl.Annotation = firstNonEmpty(root.ChildText("AnnotationText"), root.ChildText("Annotation"))
	cpl.EditRate = root.ChildText("EditRate")
	cpl.ParentRefs = collectParentRefs(root)

	for _, segNode := range root.All("Segment") {
		seg := &Segment{ID: NormalizeID(segNode.ChildText("Id"))}
		for _, seqNode := range collectSequences(segNode) {
			track := &Track{
				SequenceName: seqNode.Local,
				Kind:         classifyTrackKind(seqNode.Local),
				SequenceID:   NormalizeID(seqNode.ChildText("Id")),
				TrackID:      NormalizeID(firstNonEmpty(seqNode.ChildText("TrackId"), seqNode.ChildText("TrackID"))),
			}
			for i, resNode := range collectResources(seqNode) {
				res := &Resource{
					Index:             i,
					ID:                NormalizeID(resNode.ChildText("Id")),
					XsiType:           resNode.Attr("type"),
					TrackFileID:       NormalizeID(resNode.ChildText("TrackFileId")),
					EditRate:          resNode.ChildText("EditRate"),
					IntrinsicDuration: parseInt64(resNode.ChildText("IntrinsicDuration")),
					EntryPoint:        parseInt64(resNode.ChildText("EntryPoint")),
					SourceDuration:    parseInt64(resNode.ChildText("SourceDuration")),
					RepeatCount:       int(parseInt64(resNode.ChildText("RepeatCount"))),
					SourceEncoding:    NormalizeID(resNode.ChildText("SourceEncoding")),
				}
				if res.RepeatCount == 0 {
					res.RepeatCount = 1
				}
				if res.TrackFileID == "" {
					// Some revisions spell it TrackFileID / refer via ResourceId.
					res.TrackFileID = NormalizeID(resNode.ChildText("TrackFileID"))
				}
				track.Resources = append(track.Resources, res)
			}
			seg.Tracks = append(seg.Tracks, track)
		}
		cpl.Segments = append(cpl.Segments, seg)
	}
	return cpl, nil
}

func collectSequences(seg *Node) []*Node {
	var out []*Node
	seen := map[*Node]bool{}
	for _, n := range seg.AllAny("Sequence") {
		if strings.EqualFold(n.Local, "SequenceList") {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	// AllAny("Sequence") only matches a literal <Sequence>; real CPLs use
	// typed names such as MainImageSequence, so also match the suffix.
	var walk func(*Node)
	walk = func(cur *Node) {
		for _, c := range cur.Children {
			if strings.HasSuffix(strings.ToLower(c.Local), "sequence") &&
				!strings.EqualFold(c.Local, "SequenceList") && !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(seg)
	return out
}

func collectResources(seq *Node) []*Node {
	var out []*Node
	for _, n := range seq.AllAny("TrackFileResource", "Resource") {
		// A generic <Resource> that is not a TrackFileResource (e.g. an
		// ancillary resource) is still kept, but nodes nested inside another
		// resource are skipped.
		if len(out) > 0 {
			contained := false
			for _, r := range out {
				if nodeContains(r, n) {
					contained = true
					break
				}
			}
			if contained {
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

func nodeContains(ancestor, target *Node) bool {
	found := false
	var walk func(*Node)
	walk = func(n *Node) {
		if n == target {
			found = true
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(ancestor)
	return found
}

func collectParentRefs(root *Node) []string {
	var refs []string
	seen := map[string]bool{}
	var walk func(*Node)
	walk = func(n *Node) {
		ln := strings.ToLower(n.Local)
		if strings.Contains(ln, "parent") || strings.Contains(ln, "inherit") ||
			strings.Contains(ln, "basecpl") || strings.Contains(ln, "previouscpl") ||
			strings.Contains(ln, "sourcecpl") {
			for _, id := range ExtractUUIDRefs(n.Text) {
				if !seen[id] {
					seen[id] = true
					refs = append(refs, id)
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return refs
}

func classifyTrackKind(name string) string {
	ln := strings.ToLower(name)
	switch {
	case strings.Contains(ln, "subtitle"), strings.Contains(ln, "caption"),
		strings.Contains(ln, "timedtext"), strings.Contains(ln, "text"):
		return "subtitle"
	case strings.Contains(ln, "image"), strings.Contains(ln, "video"),
		strings.Contains(ln, "picture"):
		return "image"
	case strings.Contains(ln, "audio"), strings.Contains(ln, "sound"):
		return "audio"
	case strings.Contains(ln, "data"), strings.Contains(ln, "ancillary"),
		strings.Contains(ln, "metadata"):
		return "data"
	default:
		return "other"
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
