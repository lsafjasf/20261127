package audio

import (
	"fmt"
	"math"
	"strings"
)

// AnalysisConfig controls low-band analysis and leak interval detection.
type AnalysisConfig struct {
	DialogueChannel int
	SourceChannel   int // -1 selects the strongest correlated non-dialogue channel.

	LowCutoffHz    float64
	HighCutoffHz   float64
	DialogueLowHz  float64
	DialogueHighHz float64
	WindowSec      float64
	HopSec         float64
	MaxLagSec      float64
	MinCorrelation float64
	MinDurationSec float64
	MergeGapSec    float64
	LowFloorDBFS   float64
	DialogueGateDB float64
	LeakMarginDB   float64
	// MaxLeakBelowDialogueDB keeps low-band events within this many dB of the
	// center dialogue band, preventing low-level filter noise from becoming a
	// false leak interval.
	MaxLeakBelowDialogueDB float64
}

// DefaultAnalysisConfig returns settings suitable for 48 kHz broadcast audio.
func DefaultAnalysisConfig() AnalysisConfig {
	return AnalysisConfig{
		DialogueChannel:        -1,
		SourceChannel:          -1,
		LowCutoffHz:            20,
		HighCutoffHz:           120,
		DialogueLowHz:          150,
		DialogueHighHz:         4000,
		WindowSec:              0.100,
		HopSec:                 0.020,
		MaxLagSec:              0.020,
		MinCorrelation:         0.55,
		MinDurationSec:         0.120,
		MergeGapSec:            0.080,
		LowFloorDBFS:           -60,
		DialogueGateDB:         -60,
		LeakMarginDB:           3,
		MaxLeakBelowDialogueDB: 30,
	}
}

func (c AnalysisConfig) normalized(sampleRate int) AnalysisConfig {
	d := DefaultAnalysisConfig()
	if c.LowCutoffHz <= 0 {
		c.LowCutoffHz = d.LowCutoffHz
	}
	if c.HighCutoffHz <= 0 {
		c.HighCutoffHz = d.HighCutoffHz
	}
	if c.DialogueLowHz <= 0 {
		c.DialogueLowHz = d.DialogueLowHz
	}
	if c.DialogueHighHz <= 0 {
		c.DialogueHighHz = d.DialogueHighHz
	}
	if c.WindowSec <= 0 {
		c.WindowSec = d.WindowSec
	}
	if c.HopSec <= 0 {
		c.HopSec = d.HopSec
	}
	if c.MaxLagSec <= 0 {
		c.MaxLagSec = d.MaxLagSec
	}
	if c.MinCorrelation <= 0 {
		c.MinCorrelation = d.MinCorrelation
	}
	if c.MinDurationSec <= 0 {
		c.MinDurationSec = d.MinDurationSec
	}
	if c.MergeGapSec <= 0 {
		c.MergeGapSec = d.MergeGapSec
	}
	if c.LowFloorDBFS == 0 {
		c.LowFloorDBFS = d.LowFloorDBFS
	}
	if c.DialogueGateDB == 0 {
		c.DialogueGateDB = d.DialogueGateDB
	}
	if c.LeakMarginDB <= 0 {
		c.LeakMarginDB = d.LeakMarginDB
	}
	if c.MaxLeakBelowDialogueDB <= 0 {
		c.MaxLeakBelowDialogueDB = d.MaxLeakBelowDialogueDB
	}
	nyquist := float64(sampleRate) / 2
	if c.HighCutoffHz >= nyquist {
		c.HighCutoffHz = nyquist * 0.9
	}
	if c.DialogueHighHz >= nyquist {
		c.DialogueHighHz = nyquist * 0.9
	}
	return c
}

// ChannelSummary describes low-frequency activity for one channel.
type ChannelSummary struct {
	Index            int     `json:"index"`
	Name             string  `json:"name"`
	LowEnergyDBFS    float64 `json:"low_energy_dbfs"`
	PeakLowFrameDBFS float64 `json:"peak_low_frame_dbfs"`
	PeakLowTimeSec   float64 `json:"peak_low_time_sec"`
	DialogueActiveDB float64 `json:"low_energy_during_dialogue_dbfs"`
}

// SourceCandidate ranks a non-dialogue channel by correlation with low-band
// energy in the dialogue channel.
type SourceCandidate struct {
	Index          int     `json:"index"`
	Name           string  `json:"name"`
	Score          float64 `json:"score"`
	MeanAbsCorr    float64 `json:"mean_abs_corr"`
	P75AbsCorr     float64 `json:"p75_abs_corr"`
	P90AbsCorr     float64 `json:"p90_abs_corr"`
	StrongFraction float64 `json:"strong_fraction"`
	LagSamples     int     `json:"lag_samples"`
	LagMS          float64 `json:"lag_ms"`
	Selected       bool    `json:"selected"`
}

// LeakEvent is one contiguous interval in which low-band dialogue energy is
// correlated with the selected source channel.
type LeakEvent struct {
	Index                 int     `json:"index"`
	StartSample           int     `json:"start_sample"`
	EndSample             int     `json:"end_sample"`
	StartSec              float64 `json:"start_sec"`
	EndSec                float64 `json:"end_sec"`
	DurationSec           float64 `json:"duration_sec"`
	SourceIndex           int     `json:"source_index"`
	SourceName            string  `json:"source_name"`
	Correlation           float64 `json:"correlation"`
	LagSamples            int     `json:"lag_samples"`
	LagMS                 float64 `json:"lag_ms"`
	Gain                  float64 `json:"gain"`
	DialogueLowDBFS       float64 `json:"dialogue_low_energy_dbfs"`
	SourceLowDBFS         float64 `json:"source_low_energy_dbfs"`
	ProjectedResidualDBFS float64 `json:"projected_residual_dbfs"`
	ProjectedReductionDB  float64 `json:"projected_reduction_db"`
}

// FrameMetric is the diagnostic value at one analysis hop.
type FrameMetric struct {
	Index          int       `json:"index"`
	StartSample    int       `json:"start_sample"`
	EndSample      int       `json:"end_sample"`
	StartSec       float64   `json:"start_sec"`
	EndSec         float64   `json:"end_sec"`
	LowDBFS        []float64 `json:"low_dbfs"`
	DialogueBandDB float64   `json:"dialogue_band_dbfs"`
	DialogueActive bool      `json:"dialogue_active"`
	SourceLowDBFS  float64   `json:"source_low_dbfs"`
	Correlation    float64   `json:"correlation"`
	LagSamples     int       `json:"lag_samples"`
	Leak           bool      `json:"leak"`
}

// Report is the JSON-serializable result of Analyze.
type Report struct {
	SourceFile        string            `json:"source_file,omitempty"`
	SampleRate        int               `json:"sample_rate"`
	Channels          int               `json:"channels"`
	ChannelNames      []string          `json:"channel_names"`
	DurationSec       float64           `json:"duration_sec"`
	LowBandHz         [2]float64        `json:"low_band_hz"`
	DialogueChannel   int               `json:"dialogue_channel"`
	DialogueName      string            `json:"dialogue_name"`
	DialogueGateDB    float64           `json:"dialogue_gate_dbfs"`
	DialogueActiveSec float64           `json:"dialogue_active_sec"`
	ChannelsSummary   []ChannelSummary  `json:"channel_summary"`
	SourceIndex       int               `json:"source_channel"`
	SourceName        string            `json:"source_name"`
	SourceReason      string            `json:"source_reason"`
	Candidates        []SourceCandidate `json:"source_candidates"`
	Events            []LeakEvent       `json:"leak_events"`
	LeakTotalSec      float64           `json:"leak_total_sec"`
}

// Analysis retains filtered signals and frame diagnostics for repair/export.
type Analysis struct {
	Report       Report
	Config       AnalysisConfig
	Frames       []FrameMetric
	DialogueLow  []float64
	SourceLow    []float64
	DialogueBand []float64
}

func (r Report) String() string {
	if len(r.Events) == 0 {
		return fmt.Sprintf("no low-frequency leak detected in %s", strings.Join(r.ChannelNames, ","))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "source=%s dialogue=%s events=%d leak=%.3fs\n",
		r.SourceName, r.DialogueName, len(r.Events), r.LeakTotalSec)
	for _, e := range r.Events {
		fmt.Fprintf(&b, "  #%d %.3f-%.3fs samples %d-%d corr=%.2f lag=%d gain=%.3f\n",
			e.Index+1, e.StartSec, e.EndSec, e.StartSample, e.EndSample, e.Correlation, e.LagSamples, e.Gain)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
