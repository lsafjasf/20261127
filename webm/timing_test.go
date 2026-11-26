package webm

import "testing"

func TestTimingGapAndBackwardsPTS(t *testing.T) {
	rep := &Report{
		Tracks: []TrackReport{{TrackNumber: 1, TrackType: 1, DefaultDurationNS: 100_000_000}},
		Frames: []FrameReport{
			{Index: 1, TrackNumber: 1, PTSNS: 0},
			{Index: 2, TrackNumber: 1, PTSNS: 100_000_000},
			{Index: 3, TrackNumber: 1, PTSNS: 300_000_000},
			{Index: 4, TrackNumber: 1, PTSNS: 250_000_000},
		},
		Verdict: Verdict{ErrorCounts: map[string]int{}},
	}
	analyzeAndValidateTiming(rep, DefaultOptions())
	if rep.Timing.Gaps != 1 {
		t.Fatalf("gaps=%d timing=%+v", rep.Timing.Gaps, rep.Timing)
	}
	if rep.Timing.NonMonotonic != 1 {
		t.Fatalf("nonmonotonic=%d", rep.Timing.NonMonotonic)
	}
	codes := map[string]bool{}
	for _, f := range rep.Findings {
		codes[f.Code] = true
	}
	if !codes["pts_gap"] || !codes["non_monotonic_pts"] {
		t.Fatalf("findings=%+v", rep.Findings)
	}
}
