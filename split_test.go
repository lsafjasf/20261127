package main

import "testing"

func TestSplitByStage(t *testing.T) {
	tr := &Trace{Source: "combined.csv", Frames: []*Frame{
		{Stage: StageCapture}, {Stage: StageRecord}, {Stage: StagePlayer},
		{Stage: StageRecord},
	}}
	src := splitByStage(tr)
	if src == nil {
		t.Fatal("expected split for three stages")
	}
	if src.Capture == nil || len(src.Capture.Frames) != 1 {
		t.Fatalf("capture = %+v", src.Capture)
	}
	if src.Record == nil || len(src.Record.Frames) != 2 {
		t.Fatalf("record frames = %d, want 2", len(src.Record.Frames))
	}
	if src.Player == nil || len(src.Player.Frames) != 1 {
		t.Fatalf("player = %+v", src.Player)
	}

	one := &Trace{Frames: []*Frame{{Stage: StageRecord}, {Stage: StageRecord}}}
	if splitByStage(one) != nil {
		t.Fatal("single-stage trace must not split")
	}
}
