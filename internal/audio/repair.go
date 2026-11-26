package audio

import (
	"fmt"
	"math"
)

// RepairConfig controls targeted low-band crosstalk cancellation.
type RepairConfig struct {
	Analysis        AnalysisConfig
	FadeSec         float64
	EvidenceSamples int // Maximum onset/peak/release samples retained per event.
}

// DefaultRepairConfig returns conservative defaults.
func DefaultRepairConfig() RepairConfig {
	return RepairConfig{
		Analysis:        DefaultAnalysisConfig(),
		FadeSec:         0.010,
		EvidenceSamples: 36,
	}
}

// RepairEvent compares one before/after leak interval.
type RepairEvent struct {
	Index                  int     `json:"index"`
	StartSample            int     `json:"start_sample"`
	EndSample              int     `json:"end_sample"`
	StartSec               float64 `json:"start_sec"`
	EndSec                 float64 `json:"end_sec"`
	Gain                   float64 `json:"gain"`
	LagSamples             int     `json:"lag_samples"`
	BeforeCorrelation      float64 `json:"before_correlation"`
	AfterCorrelation       float64 `json:"after_correlation"`
	BeforeLowDBFS          float64 `json:"before_low_energy_dbfs"`
	AfterLowDBFS           float64 `json:"after_low_energy_dbfs"`
	LowEnergyReductionDB   float64 `json:"low_energy_reduction_db"`
	RemovedRMSDBFS         float64 `json:"removed_rms_dbfs"`
	DialogueBandBeforeDBFS float64 `json:"dialogue_band_before_dbfs"`
	DialogueBandAfterDBFS  float64 `json:"dialogue_band_after_dbfs"`
	DialogueBandDeltaDB    float64 `json:"dialogue_band_delta_db"`
}

// SampleEvidence is one exact sample showing the applied correction.
type SampleEvidence struct {
	EventIndex   int     `json:"event_index"`
	Region       string  `json:"region"`
	Sample       int     `json:"sample"`
	TimeSec      float64 `json:"time_sec"`
	Before       float64 `json:"dialogue_before"`
	ReferenceLow float64 `json:"source_low_aligned"`
	Removed      float64 `json:"removed"`
	After        float64 `json:"dialogue_after"`
	BeforePCM16  int     `json:"dialogue_before_pcm16"`
	RemovedPCM16 int     `json:"removed_pcm16"`
	AfterPCM16   int     `json:"dialogue_after_pcm16"`
}

// RepairVerification summarizes the complete corrected interval set.
type RepairVerification struct {
	SourceName             string  `json:"source_name"`
	DialogueName           string  `json:"dialogue_name"`
	SourceIndex            int     `json:"source_channel"`
	DialogueIndex          int     `json:"dialogue_channel"`
	RepairedSamples        int     `json:"repaired_samples"`
	LeakBeforeSec          float64 `json:"leak_before_sec"`
	LeakAfterSec           float64 `json:"leak_after_sec"`
	LowEnergyBeforeDBFS    float64 `json:"low_energy_before_dbfs"`
	LowEnergyAfterDBFS     float64 `json:"low_energy_after_dbfs"`
	LowEnergyReductionDB   float64 `json:"low_energy_reduction_db"`
	CorrelationBefore      float64 `json:"correlation_before"`
	CorrelationAfter       float64 `json:"correlation_after"`
	DialogueBandBeforeDBFS float64 `json:"dialogue_band_before_dbfs"`
	DialogueBandAfterDBFS  float64 `json:"dialogue_band_after_dbfs"`
	DialogueBandDeltaDB    float64 `json:"dialogue_band_delta_db"`
}

// RepairReport is a before/after audit package. Evidence values come from the
// encoded output decoded back to samples, not from a pre-quantization copy.
type RepairReport struct {
	Before       Report             `json:"before"`
	After        Report             `json:"after"`
	Events       []RepairEvent      `json:"repaired_events"`
	Verification RepairVerification `json:"verification"`
	Evidence     []SampleEvidence   `json:"sample_evidence"`
}

// RepairResult contains the decoded output, its exact encoded bytes, and audit
// data. Writing Encoded produces the samples represented in the report.
type RepairResult struct {
	Output  *Buffer
	Encoded []byte
	Before  *Analysis
	After   *Analysis
	Report  RepairReport
}

// Repair detects low-band crosstalk, subtracts the delay-aligned low band of
// the selected source from the dialogue channel, then re-analyzes the exact
// encoded output for before/after evidence.
func Repair(input *Buffer, cfg RepairConfig) (*RepairResult, error) {
	if cfg.FadeSec <= 0 {
		cfg.FadeSec = DefaultRepairConfig().FadeSec
	}
	if cfg.EvidenceSamples <= 0 {
		cfg.EvidenceSamples = DefaultRepairConfig().EvidenceSamples
	}
	before, err := Analyze(input, cfg.Analysis)
	if err != nil {
		return nil, err
	}
	output := cloneBuffer(input)
	if len(before.Report.Events) > 0 {
		applyCancellation(output, input, before, cfg.FadeSec)
	}
	encoded, err := EncodeWAV(output)
	if err != nil {
		return nil, fmt.Errorf("encode repaired WAV: %w", err)
	}
	decoded, err := DecodeWAV(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode repaired WAV for verification: %w", err)
	}
	afterCfg := before.Config
	afterCfg.SourceChannel = before.Report.SourceIndex
	after, err := Analyze(decoded, afterCfg)
	if err != nil {
		return nil, fmt.Errorf("verify repaired WAV: %w", err)
	}
	report := buildRepairReport(input, decoded, before, after, cfg.EvidenceSamples)
	return &RepairResult{Output: decoded, Encoded: encoded, Before: before, After: after, Report: report}, nil
}

func cloneBuffer(input *Buffer) *Buffer {
	out := &Buffer{Format: input.Format, Samples: make([][]float64, len(input.Samples))}
	for ch := range input.Samples {
		out.Samples[ch] = append([]float64(nil), input.Samples[ch]...)
	}
	return out
}

func applyCancellation(output, input *Buffer, before *Analysis, fadeSec float64) {
	sr := input.Format.SampleRate
	dialogue := before.Report.DialogueChannel
	frameCount := len(output.Samples[dialogue])
	fadeSamples := max(1, int(math.Round(fadeSec*float64(sr))))
	var coverage []bool
	if len(before.Report.Events) > 1 {
		coverage = make([]bool, frameCount)
	}
	for _, event := range before.Report.Events {
		start := max(0, event.StartSample)
		end := min(frameCount, event.EndSample)
		for i := start; i < end; i++ {
			if coverage != nil && coverage[i] {
				continue
			}
			refIndex := i - event.LagSamples
			if refIndex < 0 || refIndex >= len(before.SourceLow) {
				continue
			}
			weight := edgeWeight(i, start, end, fadeSamples)
			output.Samples[dialogue][i] -= event.Gain * before.SourceLow[refIndex] * weight
			if coverage != nil {
				coverage[i] = true
			}
		}
	}
}

func edgeWeight(i, start, end, fade int) float64 {
	if i < start || i >= end {
		return 0
	}
	inLen := min(fade, (end-start)/2)
	if inLen <= 0 {
		return 1
	}
	left := float64(i-start) / float64(inLen)
	right := float64(end-1-i) / float64(inLen)
	if left < 1 {
		return 0.5 - 0.5*math.Cos(math.Pi*left)
	}
	if right < 1 {
		return 0.5 - 0.5*math.Cos(math.Pi*right)
	}
	return 1
}
