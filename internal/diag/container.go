package diag

import (
	"fmt"
	"math"

	"seekdiag/internal/mp4"
	"seekdiag/internal/seek"
)

func inspectTracks(f *mp4.File) ([]TrackView, []Finding) {
	var views []TrackView
	var findings []Finding
	for _, t := range f.Tracks {
		view := TrackView{
			TrackID:       t.ID,
			Kind:          t.Kind(),
			Codec:         t.Codec,
			Timescale:     t.Timescale,
			DurationMS:    ticksToMS(int64(t.Duration), t.Timescale),
			SampleRate:    t.SampleRate,
			Channels:      t.ChannelCount,
			SampleCount:   len(t.Samples),
			EditListFound: len(t.Edits) > 0,
		}
		spans := t.EditSpans()
		for i, e := range t.Edits {
			ev := EditView{
				Index:           i,
				SegmentDuration: e.SegmentDuration,
				MediaTime:       e.MediaTime,
				MediaRate:       e.MediaRate,
				Empty:           e.Empty(),
			}
			if i < len(spans) {
				s := spans[i]
				ev.MovieStartMS = ticksToMS(s.MovieStart, t.MovieTimescale)
				ev.MovieEndMS = ticksToMS(s.MovieEnd, t.MovieTimescale)
				if !s.Empty && !s.Unsupported {
					ev.MediaStartMS = ticksToMS(s.MediaStart, t.Timescale)
					ev.MediaEndMS = ticksToMS(s.MediaEnd, t.Timescale)
				}
				if s.Unsupported {
					findings = append(findings, Finding{
						Layer:    VerdictContainer,
						Code:     "unsupported_edit_rate",
						Message:  fmt.Sprintf("track %d edit %d has non-unit media rate %.6f; the seek mapping is not reliable", t.ID, i, e.MediaRate),
						Severity: "error",
					})
				}
			}
			view.Edits = append(view.Edits, ev)
		}
		if len(view.Edits) == 0 {
			view.Edits = []EditView{}
		}
		if len(t.Samples) > 0 {
			for i := range t.Samples {
				if _, ok := t.MediaToMovie(t.Samples[i].PTS()); ok {
					view.FirstSample = seek.PositionAt(t, i)
					break
				}
			}
		}
		findings = append(findings, inspectTimeline(t)...)
		views = append(views, view)
	}
	return views, findings
}

func inspectTimeline(t *mp4.Track) []Finding {
	var findings []Finding
	if len(t.Samples) == 0 {
		return []Finding{{
			Layer: VerdictContainer, Code: "empty_track", Severity: "error",
			Message: fmt.Sprintf("track %d has no sample table entries", t.ID),
		}}
	}
	first := t.Samples[0]
	last := t.Samples[len(t.Samples)-1]
	firstPTS := first.PTS()
	lastEnd := last.PTS() + int64(last.Duration)
	if firstPTS < 0 {
		findings = append(findings, Finding{
			Layer: VerdictContainer, Code: "negative_pts", Severity: "error",
			Message: fmt.Sprintf("track %d starts at media PTS %d (%d ticks)", t.ID, firstPTS, firstPTS),
		})
	}
	for i := 1; i < len(t.Samples) && t.Kind() == "audio"; i++ {
		prev, cur := t.Samples[i-1], t.Samples[i]
		if cur.PTS() < prev.PTS() {
			findings = append(findings, Finding{
				Layer: VerdictContainer, Code: "pts_regression", Severity: "error",
				Message: fmt.Sprintf("track %d sample %d PTS regresses from %d to %d ticks", t.ID, cur.Index, prev.PTS(), cur.PTS()),
			})
			break
		}
	}
	spans := t.EditSpans()
	for i, s := range spans {
		if s.Empty || s.Unsupported {
			continue
		}
		if s.MediaStart < firstPTS-int64(first.Duration) || s.MediaStart > lastEnd {
			findings = append(findings, Finding{
				Layer: VerdictContainer, Code: "edit_start_outside_samples", Severity: "error",
				Message: fmt.Sprintf("track %d edit %d media start %d ticks is outside sample range [%d,%d)", t.ID, i, s.MediaStart, firstPTS, lastEnd),
			})
		}
		if s.MediaEnd > lastEnd+int64(last.Duration) {
			findings = append(findings, Finding{
				Layer: VerdictContainer, Code: "edit_end_outside_samples", Severity: "error",
				Message: fmt.Sprintf("track %d edit %d maps media end %d ticks beyond sample range end %d", t.ID, i, s.MediaEnd, lastEnd),
			})
		}
	}
	if t.Kind() == "audio" && t.SampleRate > 0 {
		ratio := float64(t.SampleRate) / float64(t.Timescale)
		if math.Abs(ratio-1) > 0.01 {
			findings = append(findings, Finding{
				Layer: VerdictContainer, Code: "audio_timescale_mismatch", Severity: "warning",
				Message: fmt.Sprintf("track %d media timescale %d differs from sample rate %d; verify PCM-position conversion", t.ID, t.Timescale, t.SampleRate),
			})
		}
		frameSamples := float64(first.Duration) * float64(t.SampleRate) / float64(t.Timescale)
		rounded := math.Round(frameSamples)
		if frameSamples > 1 && math.Abs(frameSamples-rounded) > 0.05*rounded {
			findings = append(findings, Finding{
				Layer: VerdictContainer, Code: "audio_frame_duration_inconsistent", Severity: "warning",
				Message: fmt.Sprintf("track %d first sample duration maps to %.3f PCM samples at %d Hz; sample table and audio rate disagree", t.ID, frameSamples, t.SampleRate),
			})
		}
	}
	if t.Duration > 0 {
		declared := float64(t.Duration)
		covered := float64(lastEnd)
		if math.Abs(declared-covered) > float64(last.Duration)*2 {
			findings = append(findings, Finding{
				Layer: VerdictContainer, Code: "mdhd_duration_mismatch", Severity: "warning",
				Message: fmt.Sprintf("track %d mdhd duration %.0f ticks differs from sample coverage %.0f ticks", t.ID, declared, covered),
			})
		}
	}
	return findings
}
