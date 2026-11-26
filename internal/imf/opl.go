package imf

import (
	"os"
	"strings"
)

func parseOPLFile(path, packageLabel string) (*OPL, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	root, err := ParseNode(f)
	if err != nil {
		return nil, err
	}
	opl := &OPL{SourceFile: path, PackageLabel: packageLabel}
	opl.ID = NormalizeID(root.ChildText("Id"))
	opl.Annotation = firstNonEmpty(root.ChildText("AnnotationText"), root.ChildText("Annotation"))

	for _, n := range root.AllAny("CompositionPlaylistId", "CompositionPlaylistID", "CPLId", "CPLID") {
		if id := NormalizeID(n.Text); id != "" {
			opl.CPLRefs = appendUnique(opl.CPLRefs, id)
		}
	}
	// Some OPL revisions wrap the reference in a SourceComposition element or
	// carry it as an attribute; sweep ID-ish nodes as a fallback.
	var walk func(*Node)
	walk = func(n *Node) {
		ln := strings.ToLower(n.Local)
		if strings.Contains(ln, "compositionplaylist") || ln == "cplid" || strings.HasSuffix(ln, "cplid") {
			for _, id := range ExtractUUIDRefs(n.Text) {
				opl.CPLRefs = appendUnique(opl.CPLRefs, id)
			}
			for _, a := range n.Attrs {
				if strings.Contains(strings.ToLower(a.Name.Local), "id") {
					if id := NormalizeID(a.Value); IsUUID(id) {
						opl.CPLRefs = appendUnique(opl.CPLRefs, id)
					}
				}
			}
		}
		if strings.Contains(ln, "virtualtrack") || (strings.Contains(ln, "track") && strings.HasSuffix(ln, "id")) {
			for _, id := range ExtractUUIDRefs(n.Text) {
				opl.TrackRefs = appendUnique(opl.TrackRefs, id)
			}
		}
		if strings.EqualFold(ln, "Profile") || strings.EqualFold(ln, "OutputProfile") {
			if name := n.ChildText("Name"); name != "" {
				opl.ProfileNames = append(opl.ProfileNames, name)
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return opl, nil
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
