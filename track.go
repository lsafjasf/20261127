package effectinterp

import (
	"fmt"
	"sort"
	"time"
)

// Keyframe associates a parameter value with a point in time.
type Keyframe struct {
	Time  time.Duration
	Value Value
}

// Track is an immutable keyframe sequence for one parameter.
//
// Keyframes are stably sorted by time and duplicate timestamps are collapsed:
// the last keyframe supplied for a timestamp wins. Before the first keyframe
// and after the last keyframe the nearest endpoint value is held.
type Track struct {
	kind      Kind
	keyframes []Keyframe
}

// NewTrack validates and copies keyframes into a track of the given kind.
// At least one keyframe is required. Keyframes with equal timestamps are
// deterministically reduced to the last one in input order.
func NewTrack(kind Kind, keyframes []Keyframe) (Track, error) {
	if !kind.valid() {
		return Track{}, fmt.Errorf("unknown parameter kind %v", kind)
	}
	if len(keyframes) == 0 {
		return Track{}, fmt.Errorf("%s track requires at least one keyframe", kind)
	}

	keys := append([]Keyframe(nil), keyframes...)
	for i, key := range keys {
		if key.Value.Kind != kind {
			return Track{}, fmt.Errorf(
				"keyframe %d is %v, want %v",
				i, key.Value.Kind, kind,
			)
		}
		if err := key.Value.validate(); err != nil {
			return Track{}, fmt.Errorf("keyframe %d: %w", i, err)
		}
	}

	sort.SliceStable(keys, func(i, j int) bool {
		return keys[i].Time < keys[j].Time
	})

	// Stable sorting keeps input order for equal times. Overwriting here makes
	// the last supplied duplicate the effective value at that timestamp.
	compacted := make([]Keyframe, 0, len(keys))
	for _, key := range keys {
		if len(compacted) > 0 && compacted[len(compacted)-1].Time == key.Time {
			compacted[len(compacted)-1] = key
			continue
		}
		compacted = append(compacted, key)
	}

	return Track{kind: kind, keyframes: compacted}, nil
}

func (t Track) Kind() Kind {
	return t.kind
}

// Keyframes returns a copy of the effective keyframes, with duplicate
// timestamps already collapsed.
func (t Track) Keyframes() []Keyframe {
	return append([]Keyframe(nil), t.keyframes...)
}

// At samples the track at a point in time. Values before the first keyframe
// and after the last keyframe are held. An invalid zero Track returns an error.
func (t Track) At(at time.Duration) (Value, error) {
	if !t.valid() {
		return Value{}, fmt.Errorf("track is not initialized")
	}

	first := t.keyframes[0]
	last := t.keyframes[len(t.keyframes)-1]
	if at <= first.Time {
		return t.heldValue(first.Value), nil
	}
	if at >= last.Time {
		return t.heldValue(last.Value), nil
	}

	// Find the first keyframe strictly after at. Duplicate timestamps have
	// already been collapsed, so the keyframes on either side are unique.
	rightIndex := sort.Search(len(t.keyframes), func(i int) bool {
		return t.keyframes[i].Time > at
	})
	left := t.keyframes[rightIndex-1]
	if at == left.Time {
		return t.heldValue(left.Value), nil
	}
	right := t.keyframes[rightIndex]
	progress := float64(at-left.Time) / float64(right.Time-left.Time)
	return Interpolate(t.kind, left.Value, right.Value, progress)
}

// heldValue keeps every angle sample in the normalized range, including values
// returned by endpoint holds and exact-keyframe queries.
func (t Track) heldValue(value Value) Value {
	if t.kind == KindAngle {
		return AngleValue(NormalizeAngle(value.Number))
	}
	return value
}

func (t Track) valid() bool {
	return t.kind.valid() && len(t.keyframes) > 0
}
