package ripple

import "fmt"

type MarkerPolicy string

const (
	MarkerPolicyRipple MarkerPolicy = "ripple"
	MarkerPolicyFixed  MarkerPolicy = "fixed"
)

type Edit struct {
	Index          int
	Mode           Mode
	At             Tick
	Duration       Tick
	NewClipID      string
	Label          string
	MediaIn        Tick
	TargetTrackID  string
	LinkGroup      string
	RippleTrackIDs []string
	MarkerPolicy   MarkerPolicy
}

func Apply(tl *Timeline, edit Edit, audit *Audit) (*Record, error) {
	if err := Validate(*tl); err != nil {
		return nil, fmt.Errorf("pre-edit validation: %w", err)
	}
	if edit.Mode != ModeInsert && edit.Mode != ModeOverwrite {
		return nil, fmt.Errorf("unknown edit mode %q", edit.Mode)
	}
	if edit.Duration <= 0 {
		return nil, fmt.Errorf("edit duration must be positive, got %d", edit.Duration)
	}
	if edit.At < 0 {
		return nil, fmt.Errorf("edit position must be non-negative, got %d", edit.At)
	}
	target, ok := tl.TrackByID(edit.TargetTrackID)
	if !ok {
		return nil, fmt.Errorf("unknown target track %s", edit.TargetTrackID)
	}
	if target.Locked {
		return nil, fmt.Errorf("target track %s is locked", target.ID)
	}
	if edit.At > tl.Duration() {
		return nil, fmt.Errorf("edit position %d is after timeline duration %d", edit.At, tl.Duration())
	}
	if edit.NewClipID == "" {
		edit.NewClipID = tl.uniqueID(string(edit.Mode) + "-clip")
	}

	beforeDuration := tl.Duration()
	layoutBefore, err := Recompute(*tl)
	if err != nil {
		return nil, fmt.Errorf("pre-edit layout: %w", err)
	}
	oldTransitions := make(map[string]ResolvedTransition, len(layoutBefore.Transitions))
	for _, transition := range layoutBefore.Transitions {
		oldTransitions[transition.ID] = transition
	}
	oldFades := make(map[string]ResolvedFade, len(layoutBefore.Fades))
	for _, fade := range layoutBefore.Fades {
		oldFades[fade.ID] = fade
	}

	rippleTracks, rippleIDs, warnings, err := resolveRippleTracks(tl, edit)
	if err != nil {
		return nil, err
	}
	if audit != nil {
		audit.Warnings = append(audit.Warnings, warnings...)
	}

	record := &Record{
		Index:          edit.Index,
		Mode:           edit.Mode,
		At:             edit.At,
		EditDuration:   edit.Duration,
		TargetTrackID:  edit.TargetTrackID,
		NewClipID:      edit.NewClipID,
		MarkerPolicy:   edit.MarkerPolicy,
		BeforeDuration: beforeDuration,
		RippleTrackIDs: rippleIDs,
	}

	var clipChanges []Change
	var lineage clipLineage
	if edit.Mode == ModeInsert {
		clipChanges, lineage, err = applyInsertClips(tl, edit, rippleTracks)
		record.RippleDelta = edit.Duration
	} else {
		clipChanges, lineage, err = applyOverwriteClips(tl, edit)
		record.RippleDelta = 0
	}
	if err != nil {
		return record, err
	}
	record.Changes = append(record.Changes, clipChanges...)

	transitionChanges, err := reconcileTransitions(tl, oldTransitions, lineage)
	if err != nil {
		return record, err
	}
	record.Changes = append(record.Changes, transitionChanges...)
	record.Changes = append(record.Changes, reconcileFades(tl, oldFades, lineage)...)
	record.Changes = append(record.Changes, reconcileKeyframes(tl, edit, rippleTracks)...)
	record.Changes = append(record.Changes, reconcileEnvelopes(tl, edit, rippleTracks)...)
	record.Changes = append(record.Changes, reconcileMarkers(tl, edit, audit)...)

	if err := Validate(*tl); err != nil {
		return record, fmt.Errorf("post-edit validation: %w", err)
	}

	afterDuration := tl.Duration()
	record.AfterDuration = afterDuration
	record.DurationDelta = afterDuration - beforeDuration
	record.DurationLedger = afterDuration
	record.Changes = append(record.Changes, Change{
		Kind: "duration", ID: "timeline",
		OldTime: beforeDuration, NewTime: afterDuration,
		Reason: fmt.Sprintf("%s-duration-recomputed", edit.Mode),
	})

	if audit != nil {
		audit.Records = append(audit.Records, *record)
		audit.FinalDuration = afterDuration
		audit.DurationLedger = afterDuration
	}
	return record, nil
}

func resolveRippleTracks(tl *Timeline, edit Edit) (map[string]bool, []string, []string, error) {
	set := make(map[string]bool)
	ids := make([]string, 0)
	var warnings []string

	if edit.Mode != ModeInsert {
		return set, ids, warnings, nil
	}

	if edit.RippleTrackIDs == nil {
		for _, track := range tl.Tracks {
			if track.Locked {
				warnings = append(warnings, fmt.Sprintf("locked track %s was not rippled; linked sync may require an explicit decision", track.ID))
				continue
			}
			set[track.ID] = true
			ids = append(ids, track.ID)
		}
		return set, ids, warnings, nil
	}

	for _, id := range edit.RippleTrackIDs {
		track, ok := tl.TrackByID(id)
		if !ok {
			return nil, nil, nil, fmt.Errorf("ripple track %s does not exist", id)
		}
		if track.Locked {
			return nil, nil, nil, fmt.Errorf("ripple track %s is locked", id)
		}
		set[id] = true
		ids = append(ids, id)
	}
	set[edit.TargetTrackID] = true
	if !containsString(ids, edit.TargetTrackID) {
		ids = append(ids, edit.TargetTrackID)
	}
	return set, ids, warnings, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func reconcileMarkers(tl *Timeline, edit Edit, audit *Audit) []Change {
	changes := make([]Change, 0, len(tl.Markers))
	ripple := edit.Mode == ModeInsert && edit.MarkerPolicy != MarkerPolicyFixed

	for i := range tl.Markers {
		marker := &tl.Markers[i]
		oldTime := marker.Time
		newTime := oldTime
		reason := "unchanged"

		switch {
		case edit.Mode == ModeOverwrite:
			reason = "overwrite-no-ripple"
		case !ripple:
			reason = "marker-policy-fixed"
		case oldTime >= edit.At:
			newTime = oldTime + edit.Duration
			marker.Time = newTime
			reason = "insert-ripple"
		default:
			reason = "before-insert-point"
		}

		changes = append(changes, Change{
			Kind: "marker", ID: marker.ID, Detail: marker.Label,
			OldTime: oldTime, NewTime: newTime, Reason: reason,
		})
		if audit != nil {
			audit.recordMarker(edit.Index, *marker, oldTime, newTime, reason)
		}
	}
	return changes
}
