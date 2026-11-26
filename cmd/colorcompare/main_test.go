package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/lsafjasf/20261127"
)

func TestRunReadsCSVAndEmitsJSON(t *testing.T) {
	input := `id,converter_a,converter_b
dark-small,#0E0C25,#0E0C26
light-small,#DED376,#DFD378
`
	var stdout, stderr bytes.Buffer
	code := run(
		[]string{"-json"},
		strings.NewReader(input),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run() = %d, stderr: %s", code, stderr.String())
	}

	var analysis cc.Analysis
	if err := json.Unmarshal(stdout.Bytes(), &analysis); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(analysis.Samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(analysis.Samples))
	}
	if len(analysis.Reversals) != 1 {
		t.Fatalf("got %d reversals, want 1", len(analysis.Reversals))
	}
	if analysis.Samples[0].EuclideanRank != 1 || analysis.Samples[0].DeltaE2000Rank != 2 {
		t.Fatalf("unexpected dark ranks: RGB %d, CIEDE2000 %d",
			analysis.Samples[0].EuclideanRank,
			analysis.Samples[0].DeltaE2000Rank,
		)
	}
}

func TestRunRejectsInvalidRGB(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(
		[]string{"-input", "-"},
		strings.NewReader("sample,#12345,#FFFFFF\n"),
		&stdout,
		&stderr,
	)
	if code == 0 {
		t.Fatal("run() accepted invalid RGB24")
	}
	if !strings.Contains(stderr.String(), "converter A") {
		t.Fatalf("stderr does not identify the bad column: %s", stderr.String())
	}
}
