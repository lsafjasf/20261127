package ripple

import (
	"fmt"
	"math/rand"
)

type Drag struct {
	Index         int    `json:"index"`
	At            Tick   `json:"at"`
	Duration      Tick   `json:"duration"`
	ClipID        string `json:"clipId"`
	Label         string `json:"label"`
	TargetTrackID string `json:"targetTrackId"`
}

func (d Drag) Edit(mode Mode, markerPolicy MarkerPolicy) Edit {
	return Edit{
		Index:          d.Index,
		Mode:           mode,
		At:             d.At,
		Duration:       d.Duration,
		NewClipID:      d.ClipID + "-" + string(mode),
		Label:          d.Label,
		TargetTrackID:  d.TargetTrackID,
		MarkerPolicy:   markerPolicy,
		RippleTrackIDs: nil,
	}
}

func DemoTimeline() Timeline {
	return Timeline{
		TicksPerUnit: 1,
		Tracks: []Track{
			{ID: "V1", Kind: TrackVideo},
			{ID: "A1", Kind: TrackAudio},
			{ID: "A2", Kind: TrackAudio},
		},
		Clips: []Clip{
			{ID: "v-intro", TrackID: "V1", Start: 0, Duration: 8, Label: "视频开头", LinkGroup: "linked-1"},
			{ID: "v-interview", TrackID: "V1", Start: 8, Duration: 8, Label: "采访", LinkGroup: "linked-2"},
			{ID: "v-outro", TrackID: "V1", Start: 16, Duration: 8, Label: "结尾", LinkGroup: "linked-3"},

			{ID: "a-intro", TrackID: "A1", Start: 0, Duration: 8, Label: "同期声开头", LinkGroup: "linked-1"},
			{ID: "a-interview", TrackID: "A1", Start: 8, Duration: 8, Label: "同期声采访", LinkGroup: "linked-2"},
			{ID: "a-outro", TrackID: "A1", Start: 16, Duration: 8, Label: "同期声结尾", LinkGroup: "linked-3"},

			{ID: "music-bed", TrackID: "A2", Start: 0, Duration: 24, Label: "音乐床"},
		},
		Transitions: []Transition{
			{ID: "tr-v-01", TrackID: "V1", LeftClipID: "v-intro", RightClipID: "v-interview", Duration: 2, Kind: "dissolve", Alignment: TransitionCenter},
			{ID: "tr-v-02", TrackID: "V1", LeftClipID: "v-interview", RightClipID: "v-outro", Duration: 2, Kind: "dip", Alignment: TransitionCenter},
		},
		Fades: []Fade{
			{ID: "fade-a-intro-in", ClipID: "a-intro", Kind: FadeIn, Duration: 2, Curve: "equal-power"},
			{ID: "fade-a-intro-out", ClipID: "a-intro", Kind: FadeOut, Duration: 1, Curve: "linear"},
			{ID: "fade-a-outro-out", ClipID: "a-outro", Kind: FadeOut, Duration: 3, Curve: "exponential"},
			{ID: "fade-music-in", ClipID: "music-bed", Kind: FadeIn, Duration: 2, Curve: "linear"},
			{ID: "fade-music-out", ClipID: "music-bed", Kind: FadeOut, Duration: 3, Curve: "linear"},
		},
		Keyframes: []Keyframe{
			{ID: "kf-a1-0", TrackID: "A1", Target: "track", Param: "volume", Time: 0, Value: 0.2},
			{ID: "kf-a1-4", TrackID: "A1", Target: "track", Param: "volume", Time: 4, Value: 1.0},
			{ID: "kf-a1-12", TrackID: "A1", Target: "track", Param: "volume", Time: 12, Value: 0.8},
			{ID: "kf-a1-20", TrackID: "A1", Target: "track", Param: "volume", Time: 20, Value: 0.9},
			{ID: "kf-v1-10", TrackID: "V1", Target: "track", Param: "opacity", Time: 10, Value: 1},
		},
		Envelopes: []Envelope{
			{
				ID:      "env-a2-volume",
				TrackID: "A2",
				Target:  "track:volume",
				Points: []EnvelopePoint{
					{Time: 0, Value: 0.15},
					{Time: 6, Value: 0.7},
					{Time: 14, Value: 0.6},
					{Time: 24, Value: 0.35},
				},
			},
		},
		Markers: []Marker{
			{ID: "mk-start", Time: 0, Label: "开始"},
			{ID: "mk-cut-8", Time: 8, Label: "第一个切点"},
			{ID: "mk-cut-16", Time: 16, Label: "第二个切点"},
			{ID: "mk-end", Time: 24, Label: "片尾"},
		},
	}
}

func RandomDrags(seed int64, count int, within Tick) []Drag {
	rng := rand.New(rand.NewSource(seed))
	tracks := []string{"V1", "A1", "A2"}
	drags := make([]Drag, 0, count)

	for i := 0; i < count; i++ {
		at := Tick(rng.Int63n(int64(within) + 1))
		duration := Tick(1 + rng.Intn(4))
		track := tracks[rng.Intn(len(tracks))]
		drags = append(drags, Drag{
			Index:         i,
			At:            at,
			Duration:      duration,
			ClipID:        fmt.Sprintf("drag-%02d", i+1),
			Label:         fmt.Sprintf("随机素材 %02d", i+1),
			TargetTrackID: track,
		})
	}
	return drags
}

type RunResult struct {
	Seed         int64        `json:"seed"`
	Mode         Mode         `json:"mode"`
	Drags        []Drag       `json:"drags"`
	Timeline     Timeline     `json:"timeline"`
	Audit        *Audit       `json:"audit"`
	Layout       Layout       `json:"layout"`
	SyncIssues   []SyncIssue  `json:"syncIssues,omitempty"`
	Verification Verification `json:"verification"`
}

func RunRandom(seed int64, count int, mode Mode, policy MarkerPolicy) (*RunResult, error) {
	timeline := DemoTimeline()
	drags := RandomDrags(seed, count, timeline.Duration())
	audit := newAudit(mode, seed, count, timeline.Duration())
	audit.InitMarkerTraces(timeline)

	for _, drag := range drags {
		edit := drag.Edit(mode, policy)
		if _, err := Apply(&timeline, edit, audit); err != nil {
			return nil, fmt.Errorf("%s random op %d failed: %w", mode, drag.Index+1, err)
		}
	}

	layout, err := Recompute(timeline)
	if err != nil {
		return nil, err
	}
	verification := VerifyTrace(timeline, audit)

	return &RunResult{
		Seed:         seed,
		Mode:         mode,
		Drags:        drags,
		Timeline:     timeline,
		Audit:        audit,
		Layout:       layout,
		SyncIssues:   CheckLinkedSync(timeline),
		Verification: verification,
	}, nil
}
