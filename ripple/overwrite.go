package ripple

import "fmt"

func applyOverwriteClips(tl *Timeline, edit Edit) ([]Change, clipLineage, error) {
	changes := make([]Change, 0)
	lineage := newClipLineage()
	newID := edit.NewClipID
	if newID == "" {
		newID = tl.uniqueID("overwrite-clip")
	}
	if _, exists := tl.ClipByID(newID); exists {
		return nil, lineage, fmt.Errorf("overwrite clip id %s already exists", newID)
	}

	rangeEnd := edit.At + edit.Duration
	kept := make([]Clip, 0, len(tl.Clips)+1)

	for _, clip := range tl.Clips {
		if clip.TrackID != edit.TargetTrackID || clip.End() <= edit.At || clip.Start >= rangeEnd {
			kept = append(kept, clip)
			continue
		}

		old := clip
		leftExists := old.Start < edit.At
		rightExists := old.End() > rangeEnd

		switch {
		case leftExists && rightExists:
			left := old
			left.Duration = edit.At - old.Start

			rightID := tl.fragmentID(old.ID, "right")
			right := Clip{
				ID:        rightID,
				TrackID:   old.TrackID,
				Start:     rangeEnd,
				Duration:  old.End() - rangeEnd,
				MediaIn:   old.MediaIn + (rangeEnd - old.Start),
				Label:     old.Label,
				LinkGroup: old.LinkGroup,
			}

			lineage.inEdge[old.ID] = old.ID
			lineage.outEdge[old.ID] = rightID
			kept = append(kept, left, right)
			changes = append(changes,
				Change{Kind: "clip", ID: old.ID, Detail: "overwrite-left", OldStart: old.Start, NewStart: left.Start, OldEnd: old.End(), NewEnd: left.End(), Reason: "overwrite-split"},
				Change{Kind: "clip", ID: rightID, Detail: "overwrite-right", OldStart: rangeEnd, NewStart: right.Start, OldEnd: old.End(), NewEnd: right.End(), NewClipID: rightID, Reason: "overwrite-split-no-ripple"},
			)

		case leftExists:
			left := old
			left.Duration = edit.At - old.Start
			lineage.inEdge[old.ID] = old.ID
			lineage.outEdge[old.ID] = ""
			kept = append(kept, left)
			changes = append(changes, Change{Kind: "clip", ID: old.ID, Detail: "overwrite-trim-tail", OldStart: old.Start, NewStart: left.Start, OldEnd: old.End(), NewEnd: left.End(), Reason: "overwrite-replaces-tail"})

		case rightExists:
			right := old
			right.Start = rangeEnd
			right.Duration = old.End() - rangeEnd
			right.MediaIn = old.MediaIn + (rangeEnd - old.Start)
			lineage.inEdge[old.ID] = ""
			lineage.outEdge[old.ID] = old.ID
			kept = append(kept, right)
			changes = append(changes, Change{Kind: "clip", ID: old.ID, Detail: "overwrite-trim-head", OldStart: old.Start, NewStart: right.Start, OldEnd: old.End(), NewEnd: right.End(), Reason: "overwrite-replaces-head"})

		default:
			lineage.inEdge[old.ID] = ""
			lineage.outEdge[old.ID] = ""
			changes = append(changes, Change{Kind: "clip", ID: old.ID, Detail: "overwritten", OldStart: old.Start, NewStart: edit.At, OldEnd: old.End(), NewEnd: rangeEnd, Reason: "overwrite-removes-clip"})
		}
	}

	kept = append(kept, Clip{
		ID:        newID,
		TrackID:   edit.TargetTrackID,
		Start:     edit.At,
		Duration:  edit.Duration,
		MediaIn:   edit.MediaIn,
		Label:     edit.Label,
		LinkGroup: edit.LinkGroup,
	})
	tl.Clips = kept

	changes = append(changes, Change{
		Kind:     "clip",
		ID:       newID,
		Detail:   "overwritten",
		NewStart: edit.At,
		NewEnd:   rangeEnd,
		Reason:   "overwrite-new-clip",
	})

	return changes, lineage, nil
}
