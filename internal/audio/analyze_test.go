package audio_test

import (
	"math"
	"testing"

	"lfecheck/internal/audio"
	"lfecheck/internal/fixture"
)

func TestAnalyzeLocatesLFELeakInterval(t *testing.T) {
	program := fixture.Program(48000)
	cfg := audio.DefaultAnalysisConfig()
	cfg.DialogueChannel = 2
	cfg.SourceChannel = -1
	analysis, err := audio.Analyze(program, cfg)
	if err != nil {
		t.Fatal(err)
	}
	report := analysis.Report
	if report.SourceName != "LFE" {
		t.Fatalf("source = %s, want LFE; candidates=%+v", report.SourceName, report.Candidates)
	}
	if len(report.Events) == 0 {
		t.Fatal("no leak event detected")
	}
	var start = math.Inf(1)
	var end = 0.0
	for _, event := range report.Events {
		start = math.Min(start, event.StartSec)
		end = math.Max(end, event.EndSec)
		if math.Abs(event.Correlation) < 0.90 {
			t.Errorf("event correlation %.3f is too low", event.Correlation)
		}
		if event.Gain < 0.22 || event.Gain > 0.34 {
			t.Errorf("event gain %.3f, want about %.2f", event.Gain, fixture.LeakGain)
		}
	}
	if math.Abs(start-fixture.LeakStartSec) > 0.12 {
		t.Errorf("leak start %.4fs, want %.4fs", start, fixture.LeakStartSec)
	}
	if math.Abs(end-fixture.LeakEndSec) > 0.12 {
		t.Errorf("leak end %.4fs, want %.4fs", end, fixture.LeakEndSec)
	}
}

func TestAnalyzeDoesNotLeakAfterReferenceStops(t *testing.T) {
	program := fixture.Program(48000)
	cfg := audio.DefaultAnalysisConfig()
	cfg.DialogueChannel = 2
	analysis, err := audio.Analyze(program, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range analysis.Report.Events {
		if event.StartSec < 0.70 || event.EndSec > 2.30 {
			t.Errorf("event %.3f-%.3fs extends outside LFE overlap", event.StartSec, event.EndSec)
		}
	}
}

func TestAnalyzeDoesNotReportUncorrelatedLFEPresence(t *testing.T) {
	program := fixture.Program(48000)
	for i := fixture.LeakDelay; i < len(program.Samples[2]); i++ {
		program.Samples[2][i] -= fixture.LeakGain * program.Samples[3][i-fixture.LeakDelay]
	}
	cfg := audio.DefaultAnalysisConfig()
	cfg.DialogueChannel = 2
	analysis, err := audio.Analyze(program, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Report.Events) != 0 {
		t.Fatalf("clean center channel reported %d leak events", len(analysis.Report.Events))
	}
}
