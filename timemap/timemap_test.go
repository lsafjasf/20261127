package timemap

import (
	"math/big"
	"testing"
)

func TestQuantizeRules(t *testing.T) {
	cases := []struct {
		in   string
		rule RoundingRule
		want int64
	}{
		{"1/2", RoundFloor, 0},
		{"1/2", RoundNearest, 1}, // half rounds away from zero, like math.Round
		{"1/2", RoundCeil, 1},
		{"-1/2", RoundNearest, -1},
		{"23976023/1000000", RoundFloor, 23},   // 23.976...
		{"23976023/1000000", RoundNearest, 24}, // the boundary slip
		{"48960/1", RoundFloor, 48960},
		{"48960/1", RoundNearest, 48960},
	}
	for _, c := range cases {
		r, _ := new(big.Rat).SetString(c.in)
		got := Quantize(r, c.rule)
		if got.Int64() != c.want {
			t.Errorf("Quantize(%s, %s) = %d, want %d", c.in, c.rule, got.Int64(), c.want)
		}
	}
}

func TestVideoRoundingCause(t *testing.T) {
	edit, refresh, tracks, points := DefaultScenario()
	pairs, findings := BuildPairs(tracks, edit, refresh, points)

	var f Finding
	for _, cand := range findings {
		if cand.Track == "video" {
			f = cand
		}
	}
	if f.Cause != CauseRounding || f.Stage != StageMedia {
		t.Fatalf("video: got stage=%s cause=%s, want media/%s", f.Stage, f.Cause, CauseRounding)
	}
	if f.FirstDivergence == nil || f.FirstDivergence.Label != "T1-first-frame-boundary" {
		t.Fatalf("video first divergence = %+v, want T1", f.FirstDivergence)
	}
	if f.DeltaAtFirst.Int64() != 1 {
		t.Fatalf("video delta at first divergence = %s, want 1", f.DeltaAtFirst)
	}
	// exact hand-check: floor(24000/1001 * 41708/1e6) = 0
	exact := NewRate(24000, 1001).Scale(edit.TickToSeconds(41_708))
	if Quantize(exact, RoundFloor).Int64() != 0 {
		t.Fatal("reference media index at T1 must be 0")
	}
	_ = pairs
}

func TestSubtitleWrongRateCause(t *testing.T) {
	edit, refresh, tracks, points := DefaultScenario()
	_, findings := BuildPairs(tracks, edit, refresh, points)

	var f Finding
	for _, cand := range findings {
		if cand.Track == "subtitle" {
			f = cand
		}
	}
	if f.Cause != CauseWrongRate || f.Stage != StageMedia {
		t.Fatalf("subtitle: got stage=%s cause=%s, want media/%s", f.Stage, f.Cause, CauseWrongRate)
	}
	// drift must grow over time (rate mismatch accumulates), not stay constant
	if f.DriftAtLast == nil || f.DriftAtLast.Cmp(big.NewInt(1)) <= 0 {
		t.Fatalf("subtitle drift at last point = %v, expected accumulated drift", f.DriftAtLast)
	}
	// at 60 s: floor(24*60)=1440 vs floor(24000/1001*60)=1438
	wrong := Quantize(NewRateInt(24).Scale(big.NewRat(60, 1)), RoundFloor)
	right := Quantize(NewRate(24000, 1001).Scale(big.NewRat(60, 1)), RoundFloor)
	if wrong.Int64() != 1440 || right.Int64() != 1438 {
		t.Fatalf("60s indices: wrong=%d right=%d", wrong.Int64(), right.Int64())
	}
}

func TestOffByOneDetection(t *testing.T) {
	edit := EditResolution{TicksPerSecond: 1_000_000}
	refresh := NewRateInt(60)
	rate := NewRate(24000, 1001)
	cfg := TrackConfig{
		Name:             "synthetic",
		MediaRate:        rate,
		DisplayRule:      RoundFloor,
		BuiltUsesFloat:   false,
		BuiltMediaRule:   RoundFloor,
		BuiltDisplayRule: RoundFloor,
		BuiltIndexBias:   1, // simulate 1-based numbering
	}
	points := []TimelinePoint{{Label: "origin", Tick: 0}, {Label: "one-sec", Tick: 1_000_000}}

	ref := make([]Event, len(points))
	built := make([]Event, len(points))
	for i, p := range points {
		ref[i] = RunEvent(cfg, ModeReference, edit, refresh, p)
		built[i] = RunEvent(cfg, ModeAsBuilt, edit, refresh, p)
	}
	f := Locate(cfg, edit, refresh, points, ref, built)
	if f.Cause != CauseOffByOne || f.Stage != StageMedia {
		t.Fatalf("got stage=%s cause=%s, want media/%s", f.Stage, f.Cause, CauseOffByOne)
	}
}

func TestCleanPipelineHasZeroDeltas(t *testing.T) {
	edit := EditResolution{TicksPerSecond: 1_000_000}
	refresh := NewRateInt(60)
	rate := NewRate(24000, 1001)
	cfg := TrackConfig{
		Name:             "clean",
		MediaRate:        rate,
		DisplayRule:      RoundFloor,
		BuiltUsesFloat:   false,
		BuiltMediaRule:   RoundFloor,
		BuiltDisplayRule: RoundFloor,
	}
	points := []TimelinePoint{{Tick: 0}, {Tick: 41_708}, {Tick: 60_000_000}}
	pairs, findings := BuildPairs([]TrackConfig{cfg}, edit, refresh, points)
	for _, p := range pairs[0] {
		if p.DMed.Sign() != 0 || p.DDisp.Sign() != 0 {
			t.Fatalf("clean pipeline drifted at tick %d", p.Point.Tick)
		}
	}
	if findings[0].Cause != CauseNone {
		t.Fatalf("clean pipeline cause = %s, want none", findings[0].Cause)
	}
}

func TestFormatSecondsExact(t *testing.T) {
	r, _ := new(big.Rat).SetString("1004208/48000") // 20.921 s
	if got := FormatSeconds(r, 6); got != "20.921000" {
		t.Fatalf("FormatSeconds = %q", got)
	}
}
