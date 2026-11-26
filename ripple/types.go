package ripple

import (
	"fmt"
	"sort"
)

// Tick is the timeline's integer time unit. The demo uses milliseconds,
// but tests may treat it as frames or samples.
type Tick int64

func (t Tick) String() string { return fmt.Sprintf("%dt", int64(t)) }

type Mode string

const (
	ModeInsert    Mode = "insert"
	ModeOverwrite Mode = "overwrite"
)

type TrackKind string

const (
	TrackVideo TrackKind = "video"
	TrackAudio TrackKind = "audio"
	TrackData  TrackKind = "data"
)

type Track struct {
	ID     string    `json:"id"`
	Kind   TrackKind `json:"kind"`
	Locked bool      `json:"locked,omitempty"`
}

type Clip struct {
	ID        string `json:"id"`
	TrackID   string `json:"trackId"`
	Start     Tick   `json:"start"`
	Duration  Tick   `json:"duration"`
	MediaIn   Tick   `json:"mediaIn"`
	Label     string `json:"label,omitempty"`
	LinkGroup string `json:"linkGroup,omitempty"`
}

func (c Clip) End() Tick { return c.Start + c.Duration }

type TransitionAlignment string

const (
	TransitionCenter TransitionAlignment = "center"
	TransitionEnd    TransitionAlignment = "end"
)

// Transition is anchored to a clip boundary. Its rendered Start is always
// recomputed from the two clips named by LeftClipID/RightClipID.
type Transition struct {
	ID          string              `json:"id"`
	TrackID     string              `json:"trackId"`
	LeftClipID  string              `json:"leftClipId"`
	RightClipID string              `json:"rightClipId"`
	Duration    Tick                `json:"duration"`
	Kind        string              `json:"kind,omitempty"`
	Alignment   TransitionAlignment `json:"alignment,omitempty"`
}

type ResolvedTransition struct {
	Transition
	Boundary Tick `json:"boundary"`
	Start    Tick `json:"start"`
	End      Tick `json:"end"`
}

type FadeKind string

const (
	FadeIn  FadeKind = "in"
	FadeOut FadeKind = "out"
)

type Fade struct {
	ID       string   `json:"id"`
	ClipID   string   `json:"clipId"`
	Kind     FadeKind `json:"kind"`
	Duration Tick     `json:"duration"`
	Curve    string   `json:"curve,omitempty"`
}

type Interval struct {
	Start Tick `json:"start"`
	End   Tick `json:"end"`
}

type Keyframe struct {
	ID      string  `json:"id"`
	TrackID string  `json:"trackId"`
	Target  string  `json:"target,omitempty"`
	Param   string  `json:"param"`
	Time    Tick    `json:"time"`
	Value   float64 `json:"value"`
}

type EnvelopePoint struct {
	Time  Tick    `json:"time"`
	Value float64 `json:"value"`
}

type Envelope struct {
	ID      string          `json:"id"`
	TrackID string          `json:"trackId"`
	Target  string          `json:"target,omitempty"`
	Points  []EnvelopePoint `json:"points"`
}

type Marker struct {
	ID    string `json:"id"`
	Time  Tick   `json:"time"`
	Label string `json:"label,omitempty"`
}

type Timeline struct {
	TicksPerUnit int          `json:"ticksPerUnit,omitempty"`
	Tracks       []Track      `json:"tracks"`
	Clips        []Clip       `json:"clips"`
	Transitions  []Transition `json:"transitions,omitempty"`
	Fades        []Fade       `json:"fades,omitempty"`
	Keyframes    []Keyframe   `json:"keyframes,omitempty"`
	Envelopes    []Envelope   `json:"envelopes,omitempty"`
	Markers      []Marker     `json:"markers,omitempty"`

	idSeq int `json:"-"`
}

func (tl Timeline) Duration() Tick {
	var end Tick
	for _, clip := range tl.Clips {
		if clip.End() > end {
			end = clip.End()
		}
	}
	return end
}

func (tl Timeline) ClipByID(id string) (Clip, bool) {
	for _, clip := range tl.Clips {
		if clip.ID == id {
			return clip, true
		}
	}
	return Clip{}, false
}

func (tl Timeline) TrackByID(id string) (Track, bool) {
	for _, track := range tl.Tracks {
		if track.ID == id {
			return track, true
		}
	}
	return Track{}, false
}

func sortedPoints(points []EnvelopePoint) {
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].Time != points[j].Time {
			return points[i].Time < points[j].Time
		}
		return points[i].Value < points[j].Value
	})
}
