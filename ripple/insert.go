package ripple

import "fmt"

type clipLineage struct {
	inEdge  map[string]string
	outEdge map[string]string
}

func newClipLineage() clipLineage {
	return clipLineage{
		inEdge:  make(map[string]string),
		outEdge: make(map[string]string),
	}
}

func applyInsertClips(tl *Timeline, edit Edit, rippleTracks map[string]bool) ([]Change, clipLineage, error) {
	changes := make([]Change, 0)
	lineage := newClipLineage()

	originalCount := len(tl.Clips)
	for i := 0; i < originalCount; i++ {
		clip := tl.Clips[i]
		if !rippleTracks[clip.TrackID] {
			continue
		}

		switch {
		case clip.Start >= edit.At:
			tl.Clips[i].Start += edit.Duration
			changes = append(changes, Change{
				Kind:     "clip",
				ID:       clip.ID,
				OldStart: clip.Start,
				NewStart: tl.Clips[i].Start,
				OldEnd:   clip.End(),
				NewEnd:   tl.Clips[i].End(),
				Reason:   "insert-ripple-downstream",
			})

		case clip.End() > edit.At:
			leftDuration := edit.At - clip.Start
			rightDuration := clip.End() - edit.At
			if leftDuration <= 0 || rightDuration <= 0 {
				return nil, lineage, fmt.Errorf("insert split on invalid boundary for clip %s", clip.ID)
			}

			oldEnd := clip.End()
			rightID := tl.fragmentID(clip.ID, "right")
			right := Clip{
				ID:        rightID,
				TrackID:   clip.TrackID,
				Start:     edit.At + edit.Duration,
				Duration:  rightDuration,
				MediaIn:   clip.MediaIn + leftDuration,
				Label:     clip.Label,
				LinkGroup: clip.LinkGroup,
			}

			tl.Clips[i].Duration = leftDuration
			lineage.inEdge[clip.ID] = clip.ID
			lineage.outEdge[clip.ID] = rightID
			tl.Clips = append(tl.Clips, right)

			changes = append(changes,
				Change{
					Kind:     "clip",
					ID:       clip.ID,
					Detail:   "split-left",
					OldStart: clip.Start,
					NewStart: tl.Clips[i].Start,
					OldEnd:   oldEnd,
					NewEnd:   tl.Clips[i].End(),
					Reason:   "insert-split-at-drop",
				},
				Change{
					Kind:      "clip",
					ID:        rightID,
					Detail:    "split-right-rippled",
					OldStart:  edit.At,
					NewStart:  right.Start,
					OldEnd:    edit.At + rightDuration,
					NewEnd:    right.End(),
					NewClipID: rightID,
					Reason:    "insert-split-and-ripple",
				},
			)
		}
	}

	newID := edit.NewClipID
	if newID == "" {
		newID = tl.uniqueID("inserted-clip")
	}
	if _, exists := tl.ClipByID(newID); exists {
		return nil, lineage, fmt.Errorf("inserted clip id %s already exists", newID)
	}

	inserted := Clip{
		ID:        newID,
		TrackID:   edit.TargetTrackID,
		Start:     edit.At,
		Duration:  edit.Duration,
		MediaIn:   edit.MediaIn,
		Label:     edit.Label,
		LinkGroup: edit.LinkGroup,
	}
	tl.Clips = append(tl.Clips, inserted)
	changes = append(changes, Change{
		Kind:     "clip",
		ID:       newID,
		Detail:   "inserted",
		NewStart: inserted.Start,
		NewEnd:   inserted.End(),
		Reason:   "insert-new-clip",
	})

	return changes, lineage, nil
}
