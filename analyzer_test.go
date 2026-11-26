package main

import (
	"math"
	"testing"
	"time"
)

const testSampleRate = 16000

func TestDetectsInvertedCenterContent(t *testing.T) {
	samples := testSampleRate
	left := make([]float64, samples)
	right := make([]float64, samples)

	for n := 0; n < samples; n++ {
		center := math.Sin(2 * math.Pi * 500 * float64(n) / testSampleRate)
		ambience := 0.08 * math.Sin(2*math.Pi*700*float64(n)/testSampleRate)
		left[n] = 0.7*center + ambience
		right[n] = -0.7*center + 0.08*math.Sin(2*math.Pi*700*float64(n)/testSampleRate+1.2)
	}

	report := analyze(left, right, testSampleRate, options{windowSize: 2048, hopSize: 1024})
	if report.Severity == "ok" || report.Severity == "unknown" {
		t.Fatalf("expected cancellation severity, got %q with %d segments", report.Severity, len(report.Segments))
	}
	if len(report.Segments) == 0 {
		t.Fatal("expected at least one cancellation segment")
	}
	segment := report.Segments[0]
	if segment.SeverityLevel() != "critical" {
		t.Fatalf("expected critical severity, got %q score %.3f", segment.SeverityLevel(), segment.Severity)
	}
	if segment.Evidence.CancellationDB > -15 {
		t.Fatalf("mono cancellation too weak: %.2f dB", segment.Evidence.CancellationDB)
	}
	if math.Abs(segment.Evidence.VocalPhaseDeg) < 120 {
		t.Fatalf("expected near-opposite vocal phase, got %.1f degrees", segment.Evidence.VocalPhaseDeg)
	}
	if segment.Evidence.LeftDBFS <= silenceFloorDBFS || segment.Evidence.RightDBFS <= silenceFloorDBFS {
		t.Fatal("evidence must show both stereo channels remain active")
	}
}

func TestWideSoundFieldIsNotFalsePositive(t *testing.T) {
	samples := testSampleRate
	left := make([]float64, samples)
	right := make([]float64, samples)

	for n := 0; n < samples; n++ {
		common := 0.35 * math.Sin(2*math.Pi*440*float64(n)/testSampleRate)
		left[n] = common + 0.45*math.Sin(2*math.Pi*880*float64(n)/testSampleRate)
		right[n] = common + 0.45*math.Sin(2*math.Pi*990*float64(n)/testSampleRate+0.9)
	}

	report := analyze(left, right, testSampleRate, options{windowSize: 2048, hopSize: 1024})
	if len(report.Segments) != 0 {
		t.Fatalf("wide stereo content was falsely flagged: %+v", report.Segments[0])
	}
	if report.Severity != "ok" {
		t.Fatalf("expected ok severity, got %q", report.Severity)
	}
}

func TestNinetyDegreePhaseDoesNotTriggerDisappearance(t *testing.T) {
	samples := testSampleRate
	left := make([]float64, samples)
	right := make([]float64, samples)

	for n := 0; n < samples; n++ {
		phase := 2 * math.Pi * 600 * float64(n) / testSampleRate
		left[n] = math.Sin(phase)
		right[n] = math.Sin(phase + math.Pi/2)
	}

	report := analyze(left, right, testSampleRate, options{windowSize: 2048, hopSize: 1024})
	if len(report.Segments) != 0 {
		t.Fatalf("90-degree stereo width was falsely flagged: %+v", report.Segments[0])
	}
}

func TestSegmentTimingAndWorstFrame(t *testing.T) {
	samples := testSampleRate * 2
	left := make([]float64, samples)
	right := make([]float64, samples)

	for n := 0; n < samples; n++ {
		center := math.Sin(2 * math.Pi * 520 * float64(n) / testSampleRate)
		if n >= testSampleRate/2 && n < testSampleRate {
			left[n] = center
			right[n] = -center
			continue
		}
		left[n] = center
		right[n] = center
	}

	report := analyze(left, right, testSampleRate, options{windowSize: 2048, hopSize: 1024})
	if len(report.Segments) == 0 {
		t.Fatal("expected cancellation in second half-second")
	}
	segment := report.Segments[0]
	if segment.Start < 400*time.Millisecond || segment.Start > 700*time.Millisecond {
		t.Fatalf("segment start outside expected region: %s", segment.Start)
	}
	if segment.End < 900*time.Millisecond || segment.End > 1200*time.Millisecond {
		t.Fatalf("segment end outside expected region: %s", segment.End)
	}
	if segment.WorstFrame.CancellationDB != segment.Evidence.CancellationDB {
		t.Fatal("segment evidence must come from the worst frame")
	}
}
