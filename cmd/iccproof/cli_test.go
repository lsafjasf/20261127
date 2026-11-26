package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	iccproof "iccproof"
)

func TestProofWithoutProfilesFailsInsteadOfSRGB(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"proof", "--intent", "all"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("missing profiles must fail")
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "ICC profile is required") || !strings.Contains(combined, "refusing to use a default sRGB") {
		t.Fatalf("output does not explain no-sRGB policy: %s", combined)
	}
}

func TestDemoJSONContainsPerSampleDifferencesAndBoundaries(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"demo", "--format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var report iccproof.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if len(report.Intents) != 3 || len(report.Results) == 0 {
		t.Fatalf("intents=%v results=%d", report.Intents, len(report.Results))
	}
	var foundSource, foundClip, foundDelta bool
	for _, r := range report.Results {
		if r.Boundary.Classification == iccproof.ClassSourceUnrepresentable {
			foundSource = true
		}
		if r.Boundary.Classification == iccproof.ClassDeviceClipped {
			foundClip = true
		}
		if r.DeltaE00 != nil && *r.DeltaE00 > 0 {
			foundDelta = true
		}
	}
	if !foundSource || !foundClip || !foundDelta {
		t.Fatalf("source=%v clip=%v delta=%v", foundSource, foundClip, foundDelta)
	}
}

func TestDemoCSVHasHeaderAndRows(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"demo", "--format", "csv"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	text := stdout.String()
	if !strings.HasPrefix(text, "intent,sample_index") || !strings.Contains(text, "delta_e00") {
		t.Fatalf("bad CSV prefix/header: %.300s", text)
	}
	if strings.Count(text, "\n") < 20 {
		t.Fatalf("CSV rows too short: %d bytes", len(text))
	}
}
