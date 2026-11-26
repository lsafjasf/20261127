package main

import (
	"path/filepath"
	"testing"
)

func analyzeGenerated(t *testing.T) (*Report, Sources, string) {
	t.Helper()
	dir := t.TempDir()
	gt, err := GenerateScenario(dir, 30, 12)
	if err != nil {
		t.Fatalf("GenerateScenario: %v", err)
	}
	src, warns, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected load warnings: %v", warns)
	}
	if src.Capture == nil || src.Record == nil || src.Player == nil {
		t.Fatalf("all three traces required, got %+v", src)
	}
	rep, err := Analyze(src, DefaultOptions())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	_ = gt
	return rep, src, dir
}

func findKind(rep *Report, kind string) *Finding {
	for _, f := range rep.Findings {
		if f.Kind == kind {
			return f
		}
	}
	return nil
}

func TestScenarioFirstAnomalyIsCaptureDrop(t *testing.T) {
	rep, _, _ := analyzeGenerated(t)
	if rep.First == nil {
		t.Fatal("expected a first anomaly, got nil")
	}
	if rep.First.Kind != KindCaptureDrop {
		t.Fatalf("first kind = %s, want capture_drop", rep.First.Kind)
	}
	if rep.First.Severity != "confirmed" {
		t.Fatalf("severity = %s, want confirmed", rep.First.Severity)
	}
	if d := absF(firstTime(rep.First) - 3.0); d > 0.06 {
		t.Fatalf("first time = %.3f, want ~3.0", firstTime(rep.First))
	}
	if !rep.First.HasSeq || rep.First.Seq != 90 {
		t.Fatalf("first seq = %d (%v), want 90", rep.First.Seq, rep.First.HasSeq)
	}
}

func TestScenarioClassifiesAllThreeFaults(t *testing.T) {
	rep, _, _ := analyzeGenerated(t)

	cap := findKind(rep, KindCaptureDrop)
	if cap == nil || cap.Confidence < 0.95 {
		t.Fatalf("capture_drop not confirmed: %+v", cap)
	}
	dup := findKind(rep, KindEncodeDuplicate)
	if dup == nil || dup.Severity != "confirmed" {
		t.Fatalf("encode_duplicate not confirmed: %+v", dup)
	}
	if d := absF(firstTime(dup) - 6.033); d > 0.06 {
		t.Fatalf("dup time = %.3f, want ~6.033", firstTime(dup))
	}

	skip := findKind(rep, KindPlayerSkip)
	if skip == nil || skip.Severity != "confirmed" {
		t.Fatalf("player_skip not confirmed: %+v", skip)
	}
	if skip.Frames != 4 {
		t.Fatalf("player_skip frames = %d, want 4", skip.Frames)
	}
	if skip.PTS == nil || absF(*skip.PTS-9.0) > 0.02 {
		t.Fatalf("player_skip pts = %v, want 9.0", skip.PTS)
	}
}

func TestWindowTripleValues(t *testing.T) {
	rep, _, dir := analyzeGenerated(t)
	recWins := rep.Windows["record"]
	if recWins == nil {
		t.Fatal("missing record windows")
	}
	w3 := recWins[3]
	if w3.Want != 30 || w3.Got != 27 || w3.Missing != 3 {
		t.Fatalf("w3 count triple = want %v got %d missing %v", w3.Want, w3.Got, w3.Missing)
	}
	if w3.Fresh != 27 || w3.Dup != 0 {
		t.Fatalf("w3 advancement = fresh %d dup %d, want 27/0", w3.Fresh, w3.Dup)
	}
	w6 := recWins[6]
	if w6.Got != 30 || w6.Dup != 2 || w6.Fresh != 28 {
		t.Fatalf("w6 = got %d dup %d fresh %d, want 30/2/28", w6.Got, w6.Dup, w6.Fresh)
	}

	// Record-only mode still locates the same first window.
	tr, err := LoadTraceFile(filepath.Join(dir, "record.jsonl"), StageRecord)
	if err != nil {
		t.Fatal(err)
	}
	rep2, err := Analyze(Sources{Record: tr}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if rep2.First == nil || rep2.First.Window != 3 {
		t.Fatalf("record-only first = %+v, want window 3", rep2.First)
	}
	if findKind(rep2, KindEncodeDuplicate) == nil {
		t.Fatal("record-only mode must still flag encode duplicates")
	}
	if findKind(rep2, KindPlayerSkip) != nil {
		t.Fatal("player skip must not be invented without a player trace")
	}
}

func TestCleanTraceHasNoAnomaly(t *testing.T) {
	tr := buildHashTrace(300, 30, 0, nil, nil)
	rep, err := Analyze(Sources{Record: tr}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if rep.First != nil {
		t.Fatalf("clean trace produced anomaly: %s", rep.First.Kind)
	}
}

func TestConcealedCaptureGapFilledByDuplicates(t *testing.T) {
	src := buildHashTrace(360, 30, 11, nil, nil)

	capTr := &Trace{Stage: StageCapture, Source: "cap"}
	for i, f := range src.Frames {
		if i <= 89 || i >= 93 {
			capTr.Frames = append(capTr.Frames, f)
		}
	}

	// Smooth output grid: slots 90-92 are filled with repeats of frame 89,
	// so record want==got in every window and PTS stays perfectly regular.
	recTr := &Trace{Stage: StageRecord, Source: "rec"}
	for i := 0; i < 360; i++ {
		cp := *src.Frames[i]
		cp.Stage = StageRecord
		if i >= 90 && i <= 92 {
			cp.Sig = src.Frames[89].Sig
		}
		recTr.Frames = append(recTr.Frames, &cp)
	}

	rep, err := Analyze(Sources{Capture: capTr, Record: recTr}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if rep.First == nil || rep.First.Kind != KindCaptureDrop {
		t.Fatalf("first = %+v, want capture_drop at concealed gap", rep.First)
	}
	if d := findKind(rep, KindEncodeDuplicate); d == nil || d.Severity != "confirmed" {
		t.Fatalf("filler duplicates not confirmed: %+v", d)
	}
	w3 := rep.Windows["record"][3]
	if w3.Got != 30 || w3.Dup != 3 {
		t.Fatalf("concealed w3 got=%d dup=%d, want 30/3", w3.Got, w3.Dup)
	}
}

func TestSmoothRestampedCaptureLoss(t *testing.T) {
	// Perfectly smooth frame count/PTS, but one content jump replacing
	// three missing motion steps -> suspected restamped capture loss.
	bigJump := map[int]int{60: 9}
	tr := buildHashTrace(150, 30, 0, bigJump, nil)
	rep, err := Analyze(Sources{Record: tr}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	f := findKind(rep, KindSuspectedCapture)
	if f == nil {
		t.Fatalf("expected suspected_capture_restamp, findings=%v", kinds(rep))
	}
	if absF(f.TimeS-2.0) > 0.05 {
		t.Fatalf("restamp time = %.3f, want ~2.0", f.TimeS)
	}
}

func kinds(rep *Report) []string {
	var out []string
	for _, f := range rep.Findings {
		out = append(out, f.Kind)
	}
	return out
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
