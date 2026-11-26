package tailtrace

import (
	"encoding/json"
	"os"
	"testing"
)

func TestExampleEvidenceFile(t *testing.T) {
	data, err := os.ReadFile("examples/cfr_tail_black.json")
	if err != nil {
		t.Fatal(err)
	}
	var evidence Evidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.Durations.TailBlackFrames != 2 {
		t.Fatalf("tail frames = %d", report.Durations.TailBlackFrames)
	}
	if report.OK {
		t.Fatal("example clean contract should fail until trimmed or revised")
	}
}
