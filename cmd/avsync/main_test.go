package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"avsync"
)

const cliInput = `{
  "time_unit": "ms",
  "segments": [
    {"id": "A", "source": "cam-a", "start": 0, "end": 4000},
    {"id": "B", "source": "cam-b", "start": 4000, "end": 8000}
  ],
  "pulses": [500, 1500, 2500, 3500, 4500, 5500, 6500, 7500],
  "events": [501, 1501, 2501, 3501, 4535, 5535, 6535, 7535]
}`

func TestRunJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"-json"}, strings.NewReader(cliInput), &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v; stderr=%s", err, stderr.String())
	}
	var res avsync.Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if res.FirstAnomaly == nil || res.FirstAnomaly.Source != "cam-b" {
		t.Fatalf("first anomaly = %+v", res.FirstAnomaly)
	}
	if res.After.RMSMS > 0.001 {
		t.Fatalf("repaired RMS = %.6f, want near zero", res.After.RMSMS)
	}
}

func TestRunHumanReportMentionsFirstAnomaly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(nil, strings.NewReader(cliInput), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v; stderr=%s", err, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{"首个异常源", "cam-b", "逐段修复后误差"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}
