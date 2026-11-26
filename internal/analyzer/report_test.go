package analyzer

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteJSONIsValid(t *testing.T) {
	report := Report{
		Verdict:    VerdictDisplaySampling,
		Confidence: 0.75,
		Summary:    "test",
		Source:     SequenceStats{Frames: 3},
		Scaled:     SequenceStats{Frames: 3},
		Comparison: Comparison{Scale: 1.5},
		Evidence:   []Evidence{{Code: "test", Text: "evidence"}},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, report); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if decoded["verdict"] != string(VerdictDisplaySampling) {
		t.Fatalf("verdict = %v", decoded["verdict"])
	}
}
