package tearcheck

import (
	"bytes"
	"strings"
	"testing"
)

func TestTimingAnomalyAndManualRetention(t *testing.T) {
	timings := []FrameTiming{
		{FrameIndex: 0, PTSSeconds: 0, CaptureSeconds: 100.000},
		{FrameIndex: 1, PTSSeconds: 1.0 / 30, CaptureSeconds: 100.033},
		{FrameIndex: 2, PTSSeconds: 2.0 / 30, CaptureSeconds: 100.066},
		{FrameIndex: 3, PTSSeconds: 5.0 / 30, CaptureSeconds: 100.190},
	}
	faults := timingFaultByFrame(timings)
	if faults[3] < 0.7 {
		t.Fatalf("timing fault for dropped frame = %.2f, want strong anomaly", faults[3])
	}

	report := AnalyzeObservations(nil, Metadata{FrameTiming: timings})
	if report.Summary.ManualReview == 0 {
		t.Fatalf("strong unexplained timing fault should create manual item: %+v", report.Summary)
	}
	found := false
	for _, finding := range report.ManualReviews {
		if finding.FrameIndex == 3 && finding.Cause == CauseManual {
			found = true
		}
	}
	if !found {
		t.Fatalf("frame 3 manual timing item missing: %+v", report.ManualReviews)
	}
}

func TestContentWindowResolvesObservation(t *testing.T) {
	observations := []Observation{{
		FrameIndex:     7,
		Time:           0.25,
		IntraFrameTear: 0.8,
		SeamCoverage:   0.8,
		BlockAlignment: 0.2,
	}}
	report := AnalyzeObservations(observations, Metadata{
		ContentEffectWindows: []EffectWindow{{
			StartSeconds: 0.2,
			EndSeconds:   0.4,
			Confidence:   0.95,
			Label:        "scripted glitch overlay",
		}},
	})
	if len(report.Findings) != 1 {
		t.Fatalf("findings=%d want 1", len(report.Findings))
	}
	finding := report.Findings[0]
	if finding.Cause != CauseContent || finding.ReviewRequired {
		t.Fatalf("cause=%s review=%v scores=%#v", finding.Cause, finding.ReviewRequired, finding.Scores)
	}
}

func TestJSONReportRoundTrip(t *testing.T) {
	request := Request{
		Metadata: Metadata{Source: "capture.mp4", Codec: "h264"},
		Observations: []Observation{{
			FrameIndex:         1,
			IntraFrameTear:     0.9,
			SeamCoverage:       0.9,
			AdjacentFrameMatch: 0.9,
			MatchContrast:      0.8,
			TimingFault:        0.8,
			ArbitraryScanline:  0.8,
		}},
	}
	report := AnalyzeRequest(request)
	var buffer bytes.Buffer
	if err := WriteReportJSON(&buffer, report); err != nil {
		t.Fatal(err)
	}
	jsonText := buffer.String()
	if !strings.Contains(jsonText, schemaVersion) || !strings.Contains(jsonText, "hardware_sync") {
		t.Fatalf("unexpected JSON: %s", jsonText)
	}
}

func TestOverallMixedWhenHardwareAndContentAreBothActionable(t *testing.T) {
	observations := []Observation{
		{
			FrameIndex:         10,
			SeamY:              41,
			IntraFrameTear:     0.9,
			SeamCoverage:       0.92,
			AdjacentFrameMatch: 0.94,
			MatchContrast:      0.84,
			TimingFault:        0.75,
			ArbitraryScanline:  0.86,
			BlockAlignment:     0.1,
		},
		{
			FrameIndex:         220,
			SeamY:              80,
			IntraFrameTear:     0.8,
			ContentMarker:      1,
			ChromaEffect:       0.9,
			EffectCoherence:    0.8,
			AdjacentFrameMatch: 0.1,
		},
	}
	report := AnalyzeObservations(observations, Metadata{})
	if report.Summary.Overall != CauseMixed {
		t.Fatalf("overall=%q want mixed; findings=%+v", report.Summary.Overall, report.Findings)
	}
}
