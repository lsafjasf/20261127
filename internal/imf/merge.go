package imf

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// MergeOptions controls root selection when building the final playlist.
type MergeOptions struct {
	CPLID string // explicit final CPL id (normalized ok)
	OPLID string // explicit OPL id to follow
}

// Playlist is the resolved final playlist: merged virtual tracks plus the
// OPL -> CPL -> TrackFile -> PKL/AssetMap inheritance chain.
type Playlist struct {
	OutputCPLID   string           `json:"output_cpl_id"`
	Annotation    string           `json:"annotation,omitempty"`
	EditRate      string           `json:"edit_rate,omitempty"`
	SelectedOPL   *OPLSummary      `json:"selected_opl,omitempty"`
	BaseCPL       string           `json:"base_cpl_id"`
	SourceCPLs    []CPLSummary     `json:"source_cpls"`
	Inheritance   []ChainLink      `json:"inheritance_chain"`
	Segments      []*Segment       `json:"segments"`
	Assets        []*ResolvedAsset `json:"referenced_assets"`
	TrackCount    int              `json:"track_count"`
	ResourceCount int              `json:"resource_count"`
	ResolvedCount int              `json:"resolved_count"`
	MissingCount  int              `json:"missing_count"`
}

// OPLSummary is the OPL layer of the chain.
type OPLSummary struct {
	ID           string   `json:"id"`
	PackageLabel string   `json:"package_label"`
	CPLRefs      []string `json:"cpl_refs"`
	TrackRefs    []string `json:"track_refs,omitempty"`
}

// CPLSummary records a CPL that contributed tracks.
type CPLSummary struct {
	ID           string `json:"id"`
	PackageLabel string `json:"package_label"`
	Annotation   string `json:"annotation,omitempty"`
	Role         string `json:"role,omitempty"`
}

// ChainLink is one hop in OPL -> CPL -> asset inheritance.
type ChainLink struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
	Package  string `json:"package,omitempty"`
	Path     string `json:"path,omitempty"`
	Resolved bool   `json:"resolved"`
}

// BuildPlaylist resolves inheritance and merges all partial track sets.
func (w *Workspace) BuildPlaylist(opts MergeOptions) (*Playlist, error) {
	if w.Report == nil {
		w.BuildRegistry()
	}
	opl, base, allCPLs, err := w.selectRoots(opts)
	if err != nil {
		return nil, err
	}

	pl := &Playlist{BaseCPL: base.ID, EditRate: base.EditRate, Annotation: "Merged IMF playlist"}
	if opl != nil {
		pl.SelectedOPL = &OPLSummary{
			ID: opl.ID, PackageLabel: opl.PackageLabel,
			CPLRefs: opl.CPLRefs, TrackRefs: opl.TrackRefs,
		}
		pl.Inheritance = append(pl.Inheritance, ChainLink{
			From: opl.ID, To: base.ID, Relation: "OPL references CompositionPlaylist",
			Package: opl.PackageLabel, Resolved: true,
		})
	}

	mergedSegments := []*Segment{}
	trackOrder := map[string]int{}
	orderSeq := 0

	for _, cpl := range allCPLs {
		pkgRole := w.roleOf(cpl.PackageLabel)
		pl.SourceCPLs = append(pl.SourceCPLs, CPLSummary{
			ID: cpl.ID, PackageLabel: cpl.PackageLabel,
			Annotation: cpl.Annotation, Role: pkgRole,
		})
		pl.Inheritance = append(pl.Inheritance, ChainLink{
			From: cpl.ID, Relation: fmt.Sprintf("CPL contributes %d segment(s)", len(cpl.Segments)),
			Package: cpl.PackageLabel, Resolved: true,
		})
		for _, seg := range cpl.Segments {
			target := matchSegment(mergedSegments, seg)
			if target == nil {
				target = &Segment{ID: seg.ID}
				mergedSegments = append(mergedSegments, target)
			}
			for _, tr := range seg.Tracks {
				key := trackKey(tr)
				mt := findTrack(target.Tracks, key)
				if mt == nil {
					mt = &Track{
						SequenceName: tr.SequenceName, Kind: tr.Kind,
						SequenceID: tr.SequenceID, TrackID: tr.TrackID,
					}
					target.Tracks = append(target.Tracks, mt)
					trackOrder[key] = orderSeq
					orderSeq++
				}
				mt.Provenance = appendUnique(mt.Provenance,
					fmt.Sprintf("%s:%s", cpl.PackageLabel, shortID(cpl.ID)))
				mergeResources(mt, tr)
			}
		}
	}

	// Stable canonical ordering inside each segment.
	for _, seg := range mergedSegments {
		sort.SliceStable(seg.Tracks, func(i, j int) bool {
			ri, rj := kindRank(seg.Tracks[i].Kind), kindRank(seg.Tracks[j].Kind)
			if ri != rj {
				return ri < rj
			}
			return trackOrder[trackKey(seg.Tracks[i])] < trackOrder[trackKey(seg.Tracks[j])]
		})
	}
	pl.Segments = mergedSegments

	// Resolve every resource and build the referenced-asset index.
	assetSeen := map[string]*ResolvedAsset{}
	for _, seg := range pl.Segments {
		for _, tr := range seg.Tracks {
			pl.TrackCount++
			for _, res := range tr.Resources {
				pl.ResourceCount++
				if res.TrackFileID == "" {
					continue
				}
				ra, ok := assetSeen[res.TrackFileID]
				if !ok {
					ra = w.Resolve(res.TrackFileID)
					assetSeen[res.TrackFileID] = ra
					pl.Assets = append(pl.Assets, ra)
					pl.Inheritance = append(pl.Inheritance, ChainLink{
						From: base.ID, To: res.TrackFileID,
						Relation: fmt.Sprintf("TrackFileId in %s", tr.SequenceName),
						Package:  ra.PackageLabel, Path: ra.AbsolutePath,
						Resolved: ra.Found && ra.FileExists,
					})
				}
				res.Resolved = ra
				if ra.Found && ra.FileExists {
					pl.ResolvedCount++
				} else {
					pl.MissingCount++
				}
			}
		}
	}
	sort.Slice(pl.Assets, func(i, j int) bool { return pl.Assets[i].AssetID < pl.Assets[j].AssetID })
	pl.OutputCPLID = deterministicMergedID(pl.SourceCPLs)
	return pl, nil
}

func (w *Workspace) selectRoots(opts MergeOptions) (*OPL, *CPL, []*CPL, error) {
	var allCPLs []*CPL
	var allOPLs []*OPL
	for _, p := range w.Packages {
		allCPLs = append(allCPLs, p.CPLs...)
		allOPLs = append(allOPLs, p.OPLs...)
	}
	if len(allCPLs) == 0 {
		return nil, nil, nil, fmt.Errorf("no CPL documents found in the given packages")
	}

	var opl *OPL
	if opts.OPLID != "" {
		id := NormalizeID(opts.OPLID)
		for _, o := range allOPLs {
			if o.ID == id {
				opl = o
				break
			}
		}
		if opl == nil {
			return nil, nil, nil, fmt.Errorf("OPL %s not found", opts.OPLID)
		}
	} else {
		for _, o := range allOPLs {
			for _, ref := range o.CPLRefs {
				if findCPL(allCPLs, ref) != nil {
					opl = o
					break
				}
			}
			if opl != nil {
				break
			}
		}
		if opl == nil && len(allOPLs) > 0 {
			opl = allOPLs[0]
		}
	}

	var base *CPL
	if opts.CPLID != "" {
		base = findCPL(allCPLs, NormalizeID(opts.CPLID))
		if base == nil {
			return nil, nil, nil, fmt.Errorf("CPL %s not found", opts.CPLID)
		}
	} else if opl != nil {
		for _, ref := range opl.CPLRefs {
			if c := findCPL(allCPLs, ref); c != nil {
				base = c
				break
			}
		}
	}
	if base == nil {
		base = allCPLs[0]
	}

	// Order contributions: base first, then every other CPL in package order.
	ordered := []*CPL{base}
	for _, c := range allCPLs {
		if c != base {
			ordered = append(ordered, c)
		}
	}
	return opl, base, ordered, nil
}

func (w *Workspace) roleOf(label string) string {
	for _, p := range w.Packages {
		if p.Label == label {
			return string(p.Role)
		}
	}
	return ""
}

func findCPL(cpls []*CPL, id string) *CPL {
	id = NormalizeID(id)
	for _, c := range cpls {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func matchSegment(segments []*Segment, cand *Segment) *Segment {
	if cand.ID != "" {
		for _, s := range segments {
			if s.ID == cand.ID {
				return s
			}
		}
	}
	// Common IMF shape: a single segment per CPL.
	if len(segments) == 1 && (cand.ID == "" || segments[0].ID == "") {
		return segments[0]
	}
	return nil
}

func trackKey(t *Track) string {
	if t.TrackID != "" {
		return "track:" + t.TrackID
	}
	return "seq:" + strings.ToLower(t.SequenceName)
}

func findTrack(tracks []*Track, key string) *Track {
	for _, t := range tracks {
		if trackKey(t) == key {
			return t
		}
	}
	return nil
}

func mergeResources(dst *Track, src *Track) {
	existing := map[string]bool{}
	for _, r := range dst.Resources {
		existing[resourceDedupKey(r)] = true
	}
	for _, r := range src.Resources {
		k := resourceDedupKey(r)
		if existing[k] {
			continue
		}
		existing[k] = true
		r.Index = len(dst.Resources)
		dst.Resources = append(dst.Resources, r)
	}
}

func resourceDedupKey(r *Resource) string {
	if r.ID != "" {
		return "id:" + r.ID
	}
	return fmt.Sprintf("tf:%s|in:%d|dur:%d|src:%d", r.TrackFileID, r.EntryPoint, r.IntrinsicDuration, r.SourceDuration)
}

func kindRank(kind string) int {
	switch kind {
	case "image":
		return 0
	case "audio":
		return 1
	case "subtitle":
		return 2
	case "data":
		return 3
	default:
		return 4
	}
}

func shortID(id string) string {
	id = NormalizeID(id)
	parts := strings.Split(id, "-")
	if len(parts) == 5 {
		return parts[0] + "…" + parts[4]
	}
	if len(id) > 8 {
		return id[:8] + "…" + id[len(id)-12:]
	}
	return id
}

// deterministicMergedID derives a stable UUID for the generated merged CPL.
func deterministicMergedID(srcs []CPLSummary) string {
	var b strings.Builder
	b.WriteString("imf-final-playlist-v1\n")
	for _, s := range srcs {
		b.WriteString(s.ID)
		b.WriteByte('\n')
	}
	sum := sha1.Sum([]byte(b.String()))
	raw := append([]byte{}, sum[:16]...)
	raw[6] = (raw[6] & 0x0f) | 0x50 // UUID v5-ish version nibble
	raw[8] = (raw[8] & 0x3f) | 0x80 // RFC 4122 variant
	h := hex.EncodeToString(raw)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}
