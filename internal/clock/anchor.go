package clock

import (
	"fmt"
	"math"

	"seekdiag/internal/seek"
)

type SeekAnchor struct {
	MovieMS     float64
	Audio       AudioTimeline
	AudioSample uint32
	FileOffset  uint64
	VideoSample uint32
}

// AnchorFromSeek converts the exact edited presentation target into the first
// audible PCM frame. No local PTS reset or repeated correction is involved.
func AnchorFromSeek(plan *seek.Plan) (SeekAnchor, error) {
	if plan == nil {
		return SeekAnchor{}, fmt.Errorf("nil seek plan")
	}
	anchor := SeekAnchor{MovieMS: plan.TargetMS}
	foundAudio := false
	for _, tp := range plan.Tracks {
		p := tp.Boundary.Landing
		if p == nil {
			continue
		}
		switch tp.Kind {
		case "audio":
			rate := p.SampleRate
			if rate == 0 {
				rate = p.MediaTimescale
			}
			if rate == 0 {
				return SeekAnchor{}, fmt.Errorf("audio track %d has zero sample rate", p.TrackID)
			}
			audibleFrame := p.PCMStart + int64(math.Round(p.TrimBeforeMS*float64(rate)/1000))
			anchor.Audio = AudioTimeline{
				AnchorMovieMS: plan.TargetMS,
				AnchorPCM:     audibleFrame,
				SampleRate:    rate,
			}
			anchor.AudioSample = p.SampleIndex
			anchor.FileOffset = p.FileOffset
			foundAudio = true
		case "video":
			anchor.VideoSample = p.SampleIndex
		}
	}
	if !foundAudio {
		return SeekAnchor{}, fmt.Errorf("seek plan has no audio landing")
	}
	return anchor, nil
}
