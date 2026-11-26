package effectinterp

import (
	"fmt"
	"sort"
	"time"
)

// Frame maps parameter names to their sampled values.
type Frame map[string]Value

// Scene is an immutable set of parameter tracks that can be sampled for either
// a preview or an offline render.
type Scene struct {
	names  []string
	tracks map[string]Track
}

// NewScene validates tracks and copies them into a scene. Parameter names must
// be non-empty.
func NewScene(tracks map[string]Track) (*Scene, error) {
	names := make([]string, 0, len(tracks))
	copied := make(map[string]Track, len(tracks))
	for name, track := range tracks {
		if name == "" {
			return nil, fmt.Errorf("parameter name must not be empty")
		}
		if !track.valid() {
			return nil, fmt.Errorf("parameter %q has an invalid track", name)
		}
		names = append(names, name)
		copied[name] = track
	}
	sort.Strings(names)
	return &Scene{names: names, tracks: copied}, nil
}

// ParameterNames returns parameter names in deterministic sorted order.
func (s *Scene) ParameterNames() []string {
	return append([]string(nil), s.names...)
}

// Sample evaluates every parameter at the same timestamp. Preview and offline
// rendering both delegate to this method, so they cannot diverge.
func (s *Scene) Sample(at time.Duration) (Frame, error) {
	if s == nil {
		return nil, fmt.Errorf("scene is nil")
	}
	frame := make(Frame, len(s.tracks))
	for _, name := range s.names {
		value, err := s.tracks[name].At(at)
		if err != nil {
			return nil, fmt.Errorf("parameter %q at %v: %w", name, at, err)
		}
		frame[name] = value
	}
	return frame, nil
}

// PreviewFrame samples one frame for interactive preview.
func (s *Scene) PreviewFrame(at time.Duration) (Frame, error) {
	return s.Sample(at)
}

// RenderFrame samples one frame for offline rendering.
func (s *Scene) RenderFrame(at time.Duration) (Frame, error) {
	return s.Sample(at)
}

// RenderRange samples frames from start (inclusive) through end (inclusive) at
// step intervals. It is intended for offline rendering and uses the same Sample
// path as PreviewFrame.
func (s *Scene) RenderRange(start, end, step time.Duration) ([]Frame, error) {
	if step <= 0 {
		return nil, fmt.Errorf("render step must be positive, got %v", step)
	}
	if end < start {
		return nil, fmt.Errorf("render end %v is before start %v", end, start)
	}

	frames := make([]Frame, 0)
	for at := start; ; {
		frame, err := s.Sample(at)
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
		if end-at < step {
			break
		}
		at += step
	}
	return frames, nil
}
