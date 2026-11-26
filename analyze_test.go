package avsync

import (
	"math"
	"testing"
)

func TestPerSegmentOffsetAndDrift(t *testing.T) {
	in := syntheticInput(
		[]Segment{
			{ID: "A", Source: "cam-a", Start: 0, End: 10000},
			{ID: "B", Source: "cam-b", Start: 10000, End: 20000},
		},
		[]float64{2, -48},
		[]float64{100, -250},
		500,
	)
	res, err := Analyze(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Segments) != 2 {
		t.Fatalf("got %d segments", len(res.Segments))
	}
	checkClose(t, "A offset", res.Segments[0].OffsetMS, 2, 0.05)
	checkClose(t, "A drift", res.Segments[0].DriftPPM, 100, 2)
	checkClose(t, "B offset", res.Segments[1].OffsetMS, -48, 0.05)
	checkClose(t, "B drift", res.Segments[1].DriftPPM, -250, 2)
	if res.After.MaxMS > 0.01 {
		t.Fatalf("repaired max error %.6f ms, want near zero", res.After.MaxMS)
	}
	if res.GlobalAfter.RMSMS <= res.After.RMSMS {
		t.Fatalf("global fit RMS %.3f should exceed per-segment RMS %.3f",
			res.GlobalAfter.RMSMS, res.After.RMSMS)
	}
	if res.FirstAnomaly == nil || res.FirstAnomaly.Source != "cam-b" {
		t.Fatalf("first anomaly = %+v, want cam-b", res.FirstAnomaly)
	}
}

func TestSequenceAlignmentSkipsMissingAndSpuriousEvents(t *testing.T) {
	pulses := []float64{500, 1500, 2500, 3500, 4500, 5500}
	events := make([]float64, 0, len(pulses))
	for _, p := range pulses {
		if p == 2500 {
			continue
		}
		events = append(events, p+4)
	}
	events = append(events, 3000)
	in := Input{
		TimeUnit: "ms",
		Segments: []Segment{{ID: "A", Source: "cam-a", Start: 0, End: 6000}},
		Pulses:   pulses,
		Events:   events,
	}
	res, err := Analyze(in)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Segments[0]
	if got.PairCount != 5 {
		t.Fatalf("paired %d, want 5", got.PairCount)
	}
	if res.UnmatchedPulse != 1 || res.UnmatchedEvent != 1 {
		t.Fatalf("unmatched pulses/events = %d/%d, want 1/1",
			res.UnmatchedPulse, res.UnmatchedEvent)
	}
	checkClose(t, "offset", got.OffsetMS, 4, 0.01)
}

func TestCutDiscontinuityIdentifiesLaterSource(t *testing.T) {
	in := syntheticInput(
		[]Segment{
			{ID: "A", Source: "cam-a", Start: 0, End: 8000},
			{ID: "B", Source: "cam-b", Start: 8000, End: 16000},
		},
		[]float64{1, 37},
		[]float64{0, 0},
		500,
	)
	res, err := Analyze(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Segments[0].Anomalous {
		t.Fatalf("segment A should be in tolerance: %+v", res.Segments[0])
	}
	if !res.Segments[1].Anomalous || !contains(res.Segments[1].AnomalyWhy, "cut_discontinuity") {
		t.Fatalf("segment B should have cut discontinuity: %+v", res.Segments[1])
	}
	if res.FirstAnomaly == nil || res.FirstAnomaly.SegmentID != "B" {
		t.Fatalf("first anomaly = %+v, want B", res.FirstAnomaly)
	}
}

func TestCutsAndSeconds(t *testing.T) {
	in := Input{
		TimeUnit: "s",
		Cuts:     []float64{1},
		Pulses:   []float64{0.1, 0.4, 0.7, 1.1, 1.4, 1.7},
		Events:   []float64{0.103, 0.403, 0.703, 1.143, 1.443, 1.743},
	}
	res, err := Analyze(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Segments) != 2 {
		t.Fatalf("got %d segments", len(res.Segments))
	}
	checkClose(t, "cut", res.Segments[1].StartMS, 1000, 1e-9)
	checkClose(t, "second offset", res.Segments[1].OffsetMS, 43, 0.01)
}

func syntheticInput(segs []Segment, offsets, driftsPPM []float64, stepMS float64) Input {
	in := Input{TimeUnit: "ms", Segments: segs}
	for i, s := range segs {
		for p := s.Start + stepMS; p < s.End; p += stepMS {
			x := p - s.Start
			e := offsets[i] + driftsPPM[i]*1e-6*x
			in.Pulses = append(in.Pulses, round3(p))
			in.Events = append(in.Events, round3(p+e))
		}
	}
	return in
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func checkClose(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.6f, want %.6f +/- %.6f", name, got, want, tolerance)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
