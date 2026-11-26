package ripple

import "fmt"

func reconcileTransitions(tl *Timeline, before map[string]ResolvedTransition, lineage clipLineage) ([]Change, error) {
	changes := make([]Change, 0, len(tl.Transitions))
	kept := make([]Transition, 0, len(tl.Transitions))

	for _, transition := range tl.Transitions {
		old := before[transition.ID]
		oldLeft, oldRight := transition.LeftClipID, transition.RightClipID

		if newLeft, mapped := lineage.outEdge[transition.LeftClipID]; mapped {
			transition.LeftClipID = newLeft
		}
		if newRight, mapped := lineage.inEdge[transition.RightClipID]; mapped {
			transition.RightClipID = newRight
		}

		if transition.LeftClipID == "" || transition.RightClipID == "" {
			changes = append(changes, Change{
				Kind: "transition", ID: transition.ID, OldStart: old.Start, OldEnd: old.End,
				Detail: fmt.Sprintf("%s|%s", oldLeft, oldRight), Reason: "anchor-overwritten-removed",
			})
			continue
		}
		left, leftOK := tl.ClipByID(transition.LeftClipID)
		if !leftOK {
			changes = append(changes, Change{Kind: "transition", ID: transition.ID, Reason: "missing-left-anchor-removed"})
			continue
		}
		right, rightOK := tl.ClipByID(transition.RightClipID)
		if !rightOK {
			changes = append(changes, Change{Kind: "transition", ID: transition.ID, Reason: "missing-right-anchor-removed"})
			continue
		}

		anchorNote := ""
		if left.End() != right.Start {
			if left.End() > right.Start {
				return nil, fmt.Errorf("transition %s anchors overlap after edit", transition.ID)
			}
			filler := findClipBetween(*tl, transition.TrackID, left.End(), right.Start)
			if filler == nil {
				changes = append(changes, Change{
					Kind: "transition", ID: transition.ID, OldStart: old.Start, OldEnd: old.End,
					Detail: fmt.Sprintf("%s|%s", oldLeft, oldRight),
					Reason: "ripple-gap-separates-anchors-removed",
				})
				continue
			}
			transition.LeftClipID = filler.ID
			left = *filler
			anchorNote = "boundary-retargeted-to-inserted-clip"
		}

		resolved, err := ResolveTransition(*tl, transition)
		if err != nil {
			return nil, err
		}

		reason := "unchanged"
		switch {
		case resolved.Start != old.Start:
			reason = "boundary-rippled"
		case transition.LeftClipID != oldLeft || transition.RightClipID != oldRight:
			reason = "anchor-retargeted"
		}
		if anchorNote != "" {
			reason = anchorNote
		}

		changes = append(changes, Change{
			Kind: "transition", ID: transition.ID,
			OldStart: old.Start, NewStart: resolved.Start,
			OldEnd: old.End, NewEnd: resolved.End,
			OldClipID: oldLeft, NewClipID: transition.LeftClipID,
			Detail: fmt.Sprintf("right-anchor=%s", transition.RightClipID),
			Reason: reason,
		})
		kept = append(kept, transition)
	}

	tl.Transitions = kept
	return changes, nil
}

func findClipBetween(tl Timeline, trackID string, start, end Tick) *Clip {
	for i := range tl.Clips {
		clip := &tl.Clips[i]
		if clip.TrackID == trackID && clip.Start == start && clip.End() == end {
			return clip
		}
	}
	return nil
}

func reconcileFades(tl *Timeline, before map[string]ResolvedFade, lineage clipLineage) []Change {
	changes := make([]Change, 0, len(tl.Fades))
	kept := make([]Fade, 0, len(tl.Fades))

	for _, fade := range tl.Fades {
		old := before[fade.ID]
		oldClipID := fade.ClipID

		var mapped string
		var isMapped bool
		if fade.Kind == FadeIn {
			mapped, isMapped = lineage.inEdge[fade.ClipID]
		} else {
			mapped, isMapped = lineage.outEdge[fade.ClipID]
		}
		if isMapped {
			fade.ClipID = mapped
		}

		if fade.ClipID == "" {
			changes = append(changes, Change{
				Kind: "fade", ID: fade.ID, OldClipID: oldClipID,
				OldStart: old.Interval.Start, OldEnd: old.Interval.End,
				Reason: "edge-overwritten-removed",
			})
			continue
		}

		clip, ok := tl.ClipByID(fade.ClipID)
		if !ok {
			changes = append(changes, Change{Kind: "fade", ID: fade.ID, OldClipID: oldClipID, Reason: "clip-missing-removed"})
			continue
		}

		reason := "unchanged"
		if fade.Duration > clip.Duration {
			fade.Duration = clip.Duration
			reason = "duration-clamped-to-fragment"
		}

		resolved := ResolveFade(fade, clip)
		if resolved.Interval.Start != old.Interval.Start || resolved.Interval.End != old.Interval.End {
			if reason == "unchanged" {
				reason = "fade-edge-recomputed-with-clip"
			}
		}
		if fade.ClipID != oldClipID && reason == "unchanged" {
			reason = "retargeted-to-edge-descendant"
		}

		changes = append(changes, Change{
			Kind: "fade", ID: fade.ID,
			OldStart: old.Interval.Start, NewStart: resolved.Interval.Start,
			OldEnd: old.Interval.End, NewEnd: resolved.Interval.End,
			OldClipID: oldClipID, NewClipID: fade.ClipID,
			Reason: reason,
		})
		kept = append(kept, fade)
	}

	tl.Fades = kept
	return changes
}

func reconcileKeyframes(tl *Timeline, edit Edit, rippleTracks map[string]bool) []Change {
	changes := make([]Change, 0, len(tl.Keyframes))
	for i := range tl.Keyframes {
		keyframe := &tl.Keyframes[i]
		oldTime := keyframe.Time
		newTime := oldTime
		reason := "unchanged"

		if edit.Mode == ModeInsert && rippleTracks[keyframe.TrackID] && oldTime >= edit.At {
			newTime = oldTime + edit.Duration
			keyframe.Time = newTime
			reason = "insert-ripple"
		} else if edit.Mode == ModeOverwrite {
			reason = "overwrite-no-ripple"
		}

		changes = append(changes, Change{
			Kind: "keyframe", ID: keyframe.ID,
			Detail: keyframe.Param, OldTime: oldTime, NewTime: newTime, Reason: reason,
		})
	}
	return changes
}

func reconcileEnvelopes(tl *Timeline, edit Edit, rippleTracks map[string]bool) []Change {
	changes := make([]Change, 0)

	for envelopeIndex := range tl.Envelopes {
		envelope := &tl.Envelopes[envelopeIndex]
		oldPoints := append([]EnvelopePoint(nil), envelope.Points...)
		willRipple := edit.Mode == ModeInsert && rippleTracks[envelope.TrackID]

		var valueAtCut float64
		hadExactCutPoint := false
		if willRipple {
			valueAtCut = envelopeValueAt(oldPoints, edit.At)
			for _, point := range oldPoints {
				if point.Time == edit.At {
					hadExactCutPoint = true
					break
				}
			}
			envelope.Points = rippleEnvelopePoints(oldPoints, edit.At, edit.Duration)
		}

		for _, point := range oldPoints {
			newTime := point.Time
			reason := "unchanged"
			if willRipple && point.Time >= edit.At {
				newTime = point.Time + edit.Duration
				reason = "insert-ripple"
			} else if edit.Mode == ModeOverwrite {
				reason = "overwrite-no-ripple"
			}
			changes = append(changes, Change{
				Kind: "envelope-point", ID: envelope.ID,
				Detail:  fmt.Sprintf("value=%.4g", point.Value),
				OldTime: point.Time, NewTime: newTime, Reason: reason,
			})
		}

		if willRipple {
			changes = append(changes, Change{
				Kind: "envelope-point", ID: envelope.ID,
				Detail: "split-left-hold", NewTime: edit.At,
				NewStart: edit.At, Reason: "insert-split-created-point",
			})
			if !hadExactCutPoint {
				changes = append(changes, Change{
					Kind: "envelope-point", ID: envelope.ID,
					Detail:  fmt.Sprintf("split-right-hold value=%.4g", valueAtCut),
					NewTime: edit.At + edit.Duration, NewStart: edit.At + edit.Duration,
					Reason: "insert-split-created-point",
				})
			}
		}
	}

	return changes
}

func envelopeValueAt(points []EnvelopePoint, at Tick) float64 {
	if len(points) == 0 {
		return 0
	}
	if at <= points[0].Time {
		return points[0].Value
	}
	last := points[len(points)-1]
	if at >= last.Time {
		return last.Value
	}

	for i := 1; i < len(points); i++ {
		prev, next := points[i-1], points[i]
		if at <= next.Time {
			span := next.Time - prev.Time
			if span == 0 {
				return next.Value
			}
			ratio := float64(at-prev.Time) / float64(span)
			return prev.Value + (next.Value-prev.Value)*ratio
		}
	}
	return last.Value
}

func rippleEnvelopePoints(points []EnvelopePoint, at, duration Tick) []EnvelopePoint {
	valueAtCut := envelopeValueAt(points, at)
	hadExact := false
	for _, point := range points {
		if point.Time == at {
			hadExact = true
			break
		}
	}

	result := make([]EnvelopePoint, 0, len(points)+2)
	for _, point := range points {
		if point.Time < at {
			result = append(result, point)
		} else {
			result = append(result, EnvelopePoint{Time: point.Time + duration, Value: point.Value})
		}
	}

	result = append(result, EnvelopePoint{Time: at, Value: valueAtCut})
	if !hadExact {
		result = append(result, EnvelopePoint{Time: at + duration, Value: valueAtCut})
	}
	sortEnvelopePoints(result)
	return dedupeEnvelopePoints(result)
}

func sortEnvelopePoints(points []EnvelopePoint) {
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j-1].Time > points[j].Time; j-- {
			points[j-1], points[j] = points[j], points[j-1]
		}
	}
}

func dedupeEnvelopePoints(points []EnvelopePoint) []EnvelopePoint {
	if len(points) < 2 {
		return points
	}
	result := points[:1]
	for _, point := range points[1:] {
		last := &result[len(result)-1]
		if point.Time == last.Time {
			last.Value = point.Value
			continue
		}
		result = append(result, point)
	}
	return result
}
