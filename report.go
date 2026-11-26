package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
)

type jsonReport struct {
	SampleRate  int           `json:"sample_rate"`
	Channels    int           `json:"channels"`
	DurationSec float64       `json:"duration_seconds"`
	WindowSize  int           `json:"window_size"`
	HopSize     int           `json:"hop_size"`
	Frames      int           `json:"analyzed_frames"`
	Severity    string        `json:"severity"`
	Summary     string        `json:"summary"`
	Segments    []jsonSegment `json:"segments"`
}

type jsonSegment struct {
	StartSec         float64      `json:"start_seconds"`
	EndSec           float64      `json:"end_seconds"`
	Severity         string       `json:"severity"`
	SeverityScore    float64      `json:"severity_score"`
	Frames           int          `json:"frames"`
	MeanCancellation float64      `json:"mean_cancellation_db"`
	MeanCorrelation  float64      `json:"mean_correlation"`
	MeanPhaseDeg     float64      `json:"mean_vocal_phase_degrees"`
	MeanSideMidDB    float64      `json:"mean_side_mid_ratio_db"`
	Evidence         Evidence     `json:"evidence"`
	WorstFrame       FrameMetrics `json:"worst_frame"`
}

func writeJSON(report Report, writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(toJSONReport(report))
}

func writeText(report Report, writer io.Writer) error {
	if _, err := fmt.Fprintf(writer, "Mono compatibility: %s\n", report.Severity); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Duration: %.3fs, analyzed frames: %d, FFT window: %d, hop: %d\n",
		report.Duration.Seconds(), report.Frames, report.WindowSize, report.HopSize); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s\n", report.Summary); err != nil {
		return err
	}
	for index, segment := range report.Segments {
		evidence := segment.Evidence
		if _, err := fmt.Fprintf(writer, "\n[%d] %.3f-%.3fs %s score=%.2f\n",
			index+1, segment.Start.Seconds(), segment.End.Seconds(), segment.SeverityLevel(), segment.Severity); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer,
			"  L=%5.1f dBFS  R=%5.1f dBFS  mono=%5.1f dBFS  cancellation=%5.1f dB\n",
			evidence.LeftDBFS, evidence.RightDBFS, evidence.MonoDBFS, evidence.CancellationDB); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer,
			"  correlation=%.2f  S/M=%.1f dB  vocal phase=%.1f°  vocal coherence=%.2f\n",
			evidence.Correlation, evidence.SideMidRatioDB, evidence.VocalPhaseDeg, evidence.VocalCoherence); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "  band cancellation: %s\n", formatBands(evidence.BandCancellation)); err != nil {
			return err
		}
	}
	return nil
}

func toJSONReport(report Report) jsonReport {
	segments := make([]jsonSegment, 0, len(report.Segments))
	for _, segment := range report.Segments {
		segments = append(segments, jsonSegment{
			StartSec:         segment.Start.Seconds(),
			EndSec:           segment.End.Seconds(),
			Severity:         segment.SeverityLevel(),
			SeverityScore:    round(segment.Severity),
			Frames:           segment.Frames,
			MeanCancellation: round(segment.MeanCancellation),
			MeanCorrelation:  round(segment.MeanCorrelation),
			MeanPhaseDeg:     round(wrapPhase(segment.MeanPhase) * 180 / math.Pi),
			MeanSideMidDB:    round(segment.MeanSideMid),
			Evidence:         roundEvidence(segment.Evidence),
			WorstFrame:       roundFrame(segment.WorstFrame),
		})
	}
	return jsonReport{
		SampleRate:  report.SampleRate,
		Channels:    report.Channels,
		DurationSec: round(report.Duration.Seconds()),
		WindowSize:  report.WindowSize,
		HopSize:     report.HopSize,
		Frames:      report.Frames,
		Severity:    report.Severity,
		Summary:     report.Summary,
		Segments:    segments,
	}
}

func roundFrame(frame FrameMetrics) FrameMetrics {
	frame.StartTimeSeconds = round(frame.StartTimeSeconds)
	frame.LeftRMSDB = round(frame.LeftRMSDB)
	frame.RightRMSDB = round(frame.RightRMSDB)
	frame.Correlation = round(frame.Correlation)
	frame.MonoRMSDB = round(frame.MonoRMSDB)
	frame.CancellationDB = round(frame.CancellationDB)
	frame.SideMidRatioDB = round(frame.SideMidRatioDB)
	frame.VocalPhaseRadians = round(frame.VocalPhaseRadians)
	frame.VocalCoherence = round(frame.VocalCoherence)
	frame.VocalCancellation = round(frame.VocalCancellation)
	for i := range frame.Bands {
		frame.Bands[i].Coherence = round(frame.Bands[i].Coherence)
		frame.Bands[i].WeightedPhase = round(frame.Bands[i].WeightedPhase)
		frame.Bands[i].Cancellation = round(frame.Bands[i].Cancellation)
		frame.Bands[i].MagnitudeBalance = round(frame.Bands[i].MagnitudeBalance)
	}
	return frame
}

func roundEvidence(evidence Evidence) Evidence {
	evidence.LeftDBFS = round(evidence.LeftDBFS)
	evidence.RightDBFS = round(evidence.RightDBFS)
	evidence.BalanceDB = round(evidence.BalanceDB)
	evidence.MonoDBFS = round(evidence.MonoDBFS)
	evidence.CancellationDB = round(evidence.CancellationDB)
	evidence.Correlation = round(evidence.Correlation)
	evidence.SideMidRatioDB = round(evidence.SideMidRatioDB)
	evidence.VocalPhaseDeg = round(evidence.VocalPhaseDeg)
	evidence.VocalCoherence = round(evidence.VocalCoherence)
	for name, value := range evidence.BandCancellation {
		evidence.BandCancellation[name] = round(value)
	}
	return evidence
}

func formatBands(bands map[string]float64) string {
	return fmt.Sprintf("125-250=%.2f 250-500=%.2f 500-1k=%.2f 1k-2k=%.2f 2k-4k=%.2f",
		bands["125-250"], bands["250-500"], bands["500-1k"], bands["1k-2k"], bands["2k-4k"])
}

func round(value float64) float64 {
	return math.Round(value*1000) / 1000
}
