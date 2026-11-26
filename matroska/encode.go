package matroska

import (
	"fmt"

	"mkvidentity/ebml"
	"mkvidentity/identity"
)

// ApplyTrackPatches edits TrackEntry nodes in place by TrackUID. It returns
// the UIDs that did not resolve in the Tracks tree.
func ApplyTrackPatches(tracks *ebml.Node, patches []identity.TrackPatch) []uint64 {
	var missing []uint64
	byUID := map[uint64]*ebml.Node{}
	for _, entry := range tracks.All(IDTrackEntry) {
		if uid, ok := entry.UintValue(IDTrackUID); ok {
			byUID[uid] = entry
		}
	}
	for _, patch := range patches {
		entry := byUID[patch.TargetUID]
		if entry == nil {
			missing = append(missing, patch.TargetUID)
			continue
		}
		if patch.SetLanguage {
			entry.SetString(IDLanguage, patch.Language)
		}
		if patch.SetLanguageIETF {
			entry.SetString(IDLanguageIETF, patch.LanguageIETF)
		}
		if patch.SetDefault {
			entry.SetUint(IDFlagDefault, boolUint(patch.Default))
		}
	}
	return missing
}

func boolUint(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

// ApplyChapterTrackRefMap rewrites existing ChapterTrackUID leaves in place.
// It is used when the target already owns chapter atoms and only their track
// references need identity-preserving repair.
func ApplyChapterTrackRefMap(chapters *ebml.Node, refMap map[uint64]uint64) int {
	if chapters == nil || len(refMap) == 0 {
		return 0
	}
	rewritten := 0
	var walk func(*ebml.Node)
	walk = func(node *ebml.Node) {
		if node.ID == IDChapterTrackUID && node.Children == nil {
			oldUID := ebml.ReadUint(node.Payload)
			if newUID, ok := refMap[oldUID]; ok {
				node.Payload = ebml.EncodeUint(newUID)
				rewritten++
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(chapters)
	return rewritten
}

// BuildChaptersNode groups flattened chapters by EditionUID and reconstructs
// nested ChapterAtom elements. All ChapterTrackUID values are already target
// identities in the supplied chapters.
func BuildChaptersNode(chapters []identity.Chapter) (*ebml.Node, error) {
	byEdition := map[uint64][]identity.Chapter{}
	editionOrder := []uint64{}
	for _, chapter := range chapters {
		if _, seen := byEdition[chapter.EditionUID]; !seen {
			editionOrder = append(editionOrder, chapter.EditionUID)
		}
		byEdition[chapter.EditionUID] = append(byEdition[chapter.EditionUID], chapter)
	}

	root := &ebml.Node{ID: IDChapters}
	for _, editionUID := range editionOrder {
		items := byEdition[editionUID]
		byUID := map[uint64]identity.Chapter{}
		childrenByParent := map[uint64][]identity.Chapter{}
		var top []identity.Chapter
		for _, chapter := range items {
			if chapter.UID != 0 {
				byUID[chapter.UID] = chapter
			}
			if chapter.ParentUID == 0 {
				top = append(top, chapter)
			} else {
				childrenByParent[chapter.ParentUID] = append(childrenByParent[chapter.ParentUID], chapter)
			}
		}
		for _, chapter := range items {
			if chapter.ParentUID != 0 {
				if _, ok := byUID[chapter.ParentUID]; !ok {
					return nil, fmt.Errorf("chapter %d has unresolved parent %d", chapter.UID, chapter.ParentUID)
				}
			}
		}

		edition := ebml.NewContainer(IDEditionEntry)
		if editionUID != 0 {
			edition.SetUint(IDEditionUID, editionUID)
		}
		var buildAtom func(identity.Chapter) (*ebml.Node, error)
		buildAtom = func(chapter identity.Chapter) (*ebml.Node, error) {
			if chapter.UID == 0 {
				return nil, fmt.Errorf("cannot encode chapter without ChapterUID")
			}
			if chapter.Start.NS == nil {
				return nil, fmt.Errorf("chapter %d has no encodable start time", chapter.UID)
			}
			atom := ebml.NewContainer(IDChapterAtom,
				ebml.NewUintNode(IDChapterUID, chapter.UID),
				ebml.NewUintNode(IDChapterTimeStart, *chapter.Start.NS),
			)
			if chapter.StringUID != "" {
				atom.SetString(IDChapterStringUID, chapter.StringUID)
			}
			if chapter.End != nil && chapter.End.NS != nil {
				atom.SetUint(IDChapterTimeEnd, *chapter.End.NS)
			}
			if len(chapter.TrackUIDs) > 0 {
				trackContainer := ebml.NewContainer(IDChapterTrack)
				for _, trackUID := range chapter.TrackUIDs {
					trackContainer.Children = append(trackContainer.Children,
						ebml.NewUintNode(IDChapterTrackUID, trackUID))
				}
				atom.Children = append(atom.Children, trackContainer)
			}
			for _, display := range chapter.Displays {
				node := ebml.NewContainer(IDChapterDisplay)
				if display.String != "" {
					node.SetString(IDChapString, display.String)
				}
				if display.Language != "" {
					node.SetString(IDChapLanguage, display.Language)
				}
				if display.LanguageIETF != "" {
					node.SetString(IDChapLanguageIETF, display.LanguageIETF)
				}
				atom.Children = append(atom.Children, node)
			}
			for _, child := range childrenByParent[chapter.UID] {
				childNode, err := buildAtom(child)
				if err != nil {
					return nil, err
				}
				atom.Children = append(atom.Children, childNode)
			}
			return atom, nil
		}
		for _, chapter := range top {
			atom, err := buildAtom(chapter)
			if err != nil {
				return nil, err
			}
			edition.Children = append(edition.Children, atom)
		}
		root.Children = append(root.Children, edition)
	}
	return root, nil
}
