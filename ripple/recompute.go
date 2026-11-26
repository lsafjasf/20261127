package ripple

import (
	"fmt"
	"sort"
)

type ResolvedFade struct {
	Fade
	ClipID   string   `json:"clipId"`
	Interval Interval `json:"interval"`
}

type Layout struct {
	Duration    Tick                 `json:"duration"`
	Transitions []ResolvedTransition `json:"transitions"`
	Fades       []ResolvedFade       `json:"fades"`
}

func ResolveTransition(tl Timeline, transition Transition) (ResolvedTransition, error) {
	right, ok := tl.ClipByID(transition.RightClipID)
	if !ok {
		return ResolvedTransition{}, fmt.Errorf("transition %s: missing right clip %s", transition.ID, transition.RightClipID)
	}

	boundary := right.Start
	start := boundary
	switch transition.Alignment {
	case TransitionEnd:
		start = boundary - transition.Duration
	case TransitionCenter, "":
		start = boundary - transition.Duration/2
	default:
		return ResolvedTransition{}, fmt.Errorf("transition %s: unknown alignment %q", transition.ID, transition.Alignment)
	}

	return ResolvedTransition{
		Transition: transition,
		Boundary:   boundary,
		Start:      start,
		End:        start + transition.Duration,
	}, nil
}

func ResolveFade(fade Fade, clip Clip) ResolvedFade {
	duration := fade.Duration
	if duration > clip.Duration {
		duration = clip.Duration
	}

	interval := Interval{Start: clip.Start, End: clip.Start + duration}
	if fade.Kind == FadeOut {
		interval.End = clip.End()
		interval.Start = interval.End - duration
	}

	return ResolvedFade{Fade: fade, ClipID: clip.ID, Interval: interval}
}

func Recompute(tl Timeline) (Layout, error) {
	layout := Layout{Duration: tl.Duration()}

	for _, transition := range tl.Transitions {
		resolved, err := ResolveTransition(tl, transition)
		if err != nil {
			return Layout{}, err
		}
		layout.Transitions = append(layout.Transitions, resolved)
	}

	for _, fade := range tl.Fades {
		clip, ok := tl.ClipByID(fade.ClipID)
		if !ok {
			return Layout{}, fmt.Errorf("fade %s: missing clip %s", fade.ID, fade.ClipID)
		}
		layout.Fades = append(layout.Fades, ResolveFade(fade, clip))
	}

	return layout, nil
}

func Validate(tl Timeline) error {
	trackIDs := make(map[string]bool, len(tl.Tracks))
	for _, track := range tl.Tracks {
		if track.ID == "" {
			return fmt.Errorf("track has empty id")
		}
		if trackIDs[track.ID] {
			return fmt.Errorf("duplicate track id %s", track.ID)
		}
		trackIDs[track.ID] = true
	}

	clipIDs := make(map[string]bool, len(tl.Clips))
	byTrack := make(map[string][]Clip)
	for _, clip := range tl.Clips {
		if clip.ID == "" {
			return fmt.Errorf("clip has empty id")
		}
		if clipIDs[clip.ID] {
			return fmt.Errorf("duplicate clip id %s", clip.ID)
		}
		clipIDs[clip.ID] = true
		if !trackIDs[clip.TrackID] {
			return fmt.Errorf("clip %s references missing track %s", clip.ID, clip.TrackID)
		}
		if clip.Start < 0 || clip.Duration <= 0 {
			return fmt.Errorf("clip %s has invalid start/duration %d/%d", clip.ID, clip.Start, clip.Duration)
		}
		byTrack[clip.TrackID] = append(byTrack[clip.TrackID], clip)
	}

	for trackID, clips := range byTrack {
		sort.Slice(clips, func(i, j int) bool {
			if clips[i].Start != clips[j].Start {
				return clips[i].Start < clips[j].Start
			}
			return clips[i].End() < clips[j].End()
		})
		for i := 1; i < len(clips); i++ {
			if clips[i].Start < clips[i-1].End() {
				return fmt.Errorf("track %s clips %s and %s overlap", trackID, clips[i-1].ID, clips[i].ID)
			}
		}
	}

	for _, transition := range tl.Transitions {
		if _, err := ResolveTransition(tl, transition); err != nil {
			return err
		}
		left, leftOK := tl.ClipByID(transition.LeftClipID)
		right, rightOK := tl.ClipByID(transition.RightClipID)
		if leftOK && rightOK && left.TrackID != right.TrackID {
			return fmt.Errorf("transition %s crosses tracks", transition.ID)
		}
		if leftOK && transition.TrackID != left.TrackID {
			return fmt.Errorf("transition %s track %s does not match anchor track %s",
				transition.ID, transition.TrackID, left.TrackID)
		}
		if leftOK && rightOK && left.End() != right.Start {
			return fmt.Errorf("transition %s anchors are not adjacent: %s end=%d, %s start=%d",
				transition.ID, left.ID, left.End(), right.ID, right.Start)
		}
	}

	for _, fade := range tl.Fades {
		clip, ok := tl.ClipByID(fade.ClipID)
		if !ok {
			return fmt.Errorf("fade %s references missing clip %s", fade.ID, fade.ClipID)
		}
		if fade.Duration <= 0 || fade.Duration > clip.Duration {
			return fmt.Errorf("fade %s duration %d invalid for clip %s duration %d",
				fade.ID, fade.Duration, clip.ID, clip.Duration)
		}
	}

	for _, keyframe := range tl.Keyframes {
		if keyframe.Time < 0 {
			return fmt.Errorf("keyframe %s has negative time %d", keyframe.ID, keyframe.Time)
		}
		if !trackIDs[keyframe.TrackID] {
			return fmt.Errorf("keyframe %s references missing track %s", keyframe.ID, keyframe.TrackID)
		}
	}

	for _, envelope := range tl.Envelopes {
		if !trackIDs[envelope.TrackID] {
			return fmt.Errorf("envelope %s references missing track %s", envelope.ID, envelope.TrackID)
		}
		for i, point := range envelope.Points {
			if point.Time < 0 {
				return fmt.Errorf("envelope %s point %d has negative time", envelope.ID, i)
			}
			if i > 0 && point.Time < envelope.Points[i-1].Time {
				return fmt.Errorf("envelope %s points are not sorted at index %d", envelope.ID, i)
			}
		}
	}

	for _, marker := range tl.Markers {
		if marker.Time < 0 {
			return fmt.Errorf("marker %s has negative time %d", marker.ID, marker.Time)
		}
	}

	return nil
}
