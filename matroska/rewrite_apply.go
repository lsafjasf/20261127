package matroska

import (
	"encoding/binary"
	"fmt"

	"mkvidentity/ebml"
	"mkvidentity/identity"
)

func applyPatchToElements(elements []*topElement, patch identity.Patch, result *RewriteResult) ([]*topElement, error) {
	trackPatchCount := len(patch.TrackPatches)
	for _, element := range elements {
		if element.header.ID != IDTracks || element.node == nil {
			continue
		}
		missing := ApplyTrackPatches(element.node, patch.TrackPatches)
		result.MissingTrackUIDs = append(result.MissingTrackUIDs, missing...)
		result.TracksPatched += trackPatchCount - len(missing)
		element.dirty = true
	}
	if len(patch.TrackPatches) > 0 && !hasElementID(elements, IDTracks) {
		return nil, fmt.Errorf("patch contains %d track updates but file has no Tracks element", len(patch.TrackPatches))
	}

	chapterReplaced := false
	if patch.ReplaceChapters && len(patch.Chapters) > 0 {
		chapterNode, err := BuildChaptersNode(patch.Chapters)
		if err != nil {
			return nil, fmt.Errorf("encode migrated chapters: %w", err)
		}
		for _, element := range elements {
			if element.header.ID == IDChapters {
				element.node = chapterNode
				element.dirty = true
				element.replacement = nil
				chapterReplaced = true
				result.ChaptersReplaced = len(patch.Chapters)
				break
			}
		}
		if !chapterReplaced {
			newElement := &topElement{
				header: ebml.Header{ID: IDChapters}, node: chapterNode,
				dirty: true, inserted: true,
			}
			insertAt := 0
			for i, element := range elements {
				if element.header.ID == IDTracks {
					insertAt = i + 1
					break
				}
				if element.header.ID == IDInfo {
					insertAt = i + 1
				}
			}
			elements = insertTopElement(elements, insertAt, newElement)
			result.ChaptersReplaced = len(patch.Chapters)
		}
	}
	if !chapterReplaced && len(patch.TrackRefMap) > 0 {
		for _, element := range elements {
			if element.header.ID != IDChapters || element.node == nil {
				continue
			}
			rewritten := ApplyChapterTrackRefMap(element.node, patch.TrackRefMap)
			if rewritten > 0 {
				element.dirty = true
				result.ChapterRefsRewritten += rewritten
			}
		}
	}
	return elements, nil
}

func insertTopElement(elements []*topElement, index int, element *topElement) []*topElement {
	if index >= len(elements) {
		return append(elements, element)
	}
	elements = append(elements, nil)
	copy(elements[index+1:], elements[index:])
	elements[index] = element
	return elements
}

func hasElementID(elements []*topElement, id ebml.ID) bool {
	for _, element := range elements {
		if element.header.ID == id {
			return true
		}
	}
	return false
}

type elementLayout struct {
	lengths       []int64
	newRelative   []int64
	payloadLength int64
}

func outputLength(element *topElement) int64 {
	switch {
	case len(element.replacement) > 0:
		return int64(len(element.replacement))
	case element.node != nil && (element.dirty || element.inserted):
		return int64(len(element.node.Bytes()))
	case element.inserted && element.node != nil:
		return int64(len(element.node.Bytes()))
	default:
		return element.end - element.headerStart
	}
}

func layoutElements(elements []*topElement, segmentDataStart int64) elementLayout {
	layout := elementLayout{lengths: make([]int64, len(elements)), newRelative: make([]int64, len(elements))}
	var cursor int64
	for i, element := range elements {
		layout.newRelative[i] = cursor
		length := outputLength(element)
		layout.lengths[i] = length
		cursor += length
	}
	layout.payloadLength = cursor
	_ = segmentDataStart
	return layout
}

func prepareIndexUpdates(elements []*topElement) error {
	for _, element := range elements {
		switch element.header.ID {
		case IDSeekHead:
			for _, seek := range element.node.All(IDSeek) {
				setFixedUint(seek, IDSeekPosition, 0)
			}
			element.dirty = true
		case IDCues:
			for _, point := range element.node.All(IDCuePoint) {
				for _, positions := range point.All(IDCueTrackPositions) {
					oldPosition, _ := positions.UintValue(IDCueClusterPosition)
					setFixedUint(positions, IDCueClusterPosition, oldPosition)
				}
			}
			element.dirty = true
		}
	}
	return nil
}

func updateIndexPositions(elements []*topElement, layout elementLayout, segmentDataStart int64, result *RewriteResult) {
	byID := map[ebml.ID]int{}
	byOriginalRelative := map[uint64]int{}
	for i, element := range elements {
		byID[element.header.ID] = i
		if !element.inserted {
			relative := uint64(element.headerStart - segmentDataStart)
			byOriginalRelative[relative] = i
		}
	}

	for _, element := range elements {
		if element.header.ID != IDSeekHead || element.node == nil {
			continue
		}
		for _, seek := range element.node.All(IDSeek) {
			idNode := seek.First(IDSeekID)
			posNode := seek.First(IDSeekPosition)
			if idNode == nil || posNode == nil {
				continue
			}
			targetID := ebml.ID(idFromBytes(idNode.Payload))
			index, ok := byID[targetID]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("SeekHead points to unknown element %x", targetID))
				continue
			}
			setFixedUint(seek, IDSeekPosition, uint64(layout.newRelative[index]))
			result.SeekEntriesFixed++
		}
	}

	for _, element := range elements {
		if element.header.ID != IDCues || element.node == nil {
			continue
		}
		for _, point := range element.node.All(IDCuePoint) {
			for _, positions := range point.All(IDCueTrackPositions) {
				oldPosition, ok := positions.UintValue(IDCueClusterPosition)
				if !ok {
					continue
				}
				index, ok := byOriginalRelative[oldPosition]
				if !ok {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("CueClusterPosition %d does not match a top-level element start", oldPosition))
					continue
				}
				setFixedUint(positions, IDCueClusterPosition, uint64(layout.newRelative[index]))
				result.CuePositionsFixed++
			}
		}
	}
}

func idFromBytes(payload []byte) uint32 {
	padded := make([]byte, 4)
	copy(padded[4-len(payload):], payload)
	return binary.BigEndian.Uint32(padded)
}

func setFixedUint(parent *ebml.Node, id ebml.ID, value uint64) {
	child := parent.First(id)
	if child == nil {
		child = &ebml.Node{ID: id}
		parent.Children = append(parent.Children, child)
	}
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	child.Payload = raw[:]
	child.Children = nil
}
