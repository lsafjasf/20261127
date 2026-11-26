package main

import "testing"

func TestSilentInputIsUnknown(t *testing.T) {
	samples := make([]float64, testSampleRate)
	report := analyze(samples, samples, testSampleRate, options{windowSize: 2048, hopSize: 1024})
	if report.Severity != "unknown" || report.Frames != 0 {
		t.Fatalf("silent input should be unknown, got severity=%q frames=%d", report.Severity, report.Frames)
	}
}

func TestInvalidWindowIsRejected(t *testing.T) {
	samples := make([]float64, testSampleRate)
	for i := range samples {
		samples[i] = 0.5
	}
	report := analyze(samples, samples, testSampleRate, options{windowSize: 1000, hopSize: 500})
	if report.Severity != "unknown" {
		t.Fatalf("invalid window should be unknown, got %q", report.Severity)
	}
}

func TestMismatchedChannelLengthsAreRejected(t *testing.T) {
	left := make([]float64, 1024)
	right := make([]float64, 1023)
	report := analyze(left, right, testSampleRate, options{windowSize: 1024, hopSize: 512})
	if report.Severity != "unknown" {
		t.Fatalf("mismatched channels should be unknown, got %q", report.Severity)
	}
}
