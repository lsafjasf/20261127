package gamutcompare_test

import (
	"encoding/json"
	"testing"

	gamutcompare "gamutcompare"
)

func TestSampleInputProducesJSONReport(t *testing.T) {
	displays, target, options, err := gamutcompare.ParseInput(gamutcompare.SampleInputJSON)
	if err != nil {
		t.Fatal(err)
	}
	report, err := gamutcompare.Analyze(displays, target, options)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if decoded["comparison"] == nil || decoded["displays"] == nil {
		t.Fatal("JSON report misses comparison or displays")
	}
}

func TestParseInputRejectsMissingDisplays(t *testing.T) {
	_, _, _, err := gamutcompare.ParseInput([]byte(`{"target":"srgb"}`))
	if err == nil {
		t.Fatal("expected missing displays error")
	}
}
