package iccproof

import "time"

const ReportSchemaVersion = "iccproof/v1"

// ProofConfig controls quantization and strict boundary behavior.
type ProofConfig struct {
	OutputBitDepth         int     `json:"output_bit_depth,omitempty"`
	Tolerance              float64 `json:"tolerance,omitempty"`
	CheckSourceGamut       bool    `json:"check_source_gamut,omitempty"`
	Engine                 string  `json:"engine,omitempty"`
	IncludeRawDeviceValues bool    `json:"include_raw_device_values,omitempty"`
}

func (c ProofConfig) withDefaults() ProofConfig {
	if c.OutputBitDepth == 0 {
		c.OutputBitDepth = 8
	}
	if c.Tolerance == 0 {
		c.Tolerance = 1e-4
	}
	if c.Engine == "" {
		c.Engine = "auto"
	}
	c.CheckSourceGamut = true
	return c
}

// Request is the serializable proofing input.
type Request struct {
	SchemaVersion string      `json:"schema_version,omitempty"`
	PhotoPath     string      `json:"photo_path,omitempty"`
	Source        ProfileRef  `json:"source_profile"`
	Media         ProfileRef  `json:"media_profile"`
	Display       *ProfileRef `json:"display_profile,omitempty"`
	Intents       []Intent    `json:"intents,omitempty"`
	Samples       []Sample    `json:"samples,omitempty"`
	SampleGrid    *SampleGrid `json:"sample_grid,omitempty"`
	Config        ProofConfig `json:"config,omitempty"`

	// Engine allows tests or embedders to inject a deterministic backend.
	// It is not serialized.
	Engine Engine `json:"-"`
}

// SampleGrid asks image inputs to be sampled on a regular grid.
type SampleGrid struct {
	Stride     int `json:"stride,omitempty"`
	MaxSamples int `json:"max_samples,omitempty"`
}

// SampleResult is one sample under one intent: the required per-sample ΔE row.
type SampleResult struct {
	Intent      Intent `json:"intent"`
	IntentICC   uint32 `json:"intent_icc"`
	SampleIndex int    `json:"sample_index"`
	SampleID    string `json:"sample_id,omitempty"`
	X           *int   `json:"x,omitempty"`
	Y           *int   `json:"y,omitempty"`

	InputRGB  []float64 `json:"input_rgb,omitempty"`
	InputLab  *Lab      `json:"input_lab,omitempty"`
	SourceLab *Lab      `json:"source_lab,omitempty"`

	MediaColorSpace string    `json:"media_color_space,omitempty"`
	MediaValues     []float64 `json:"media_values,omitempty"`
	MediaRawValues  []float64 `json:"media_raw_values,omitempty"`
	MediaEncoded    []float64 `json:"media_encoded,omitempty"`
	MediaCodes      []int     `json:"media_codes,omitempty"`
	MediaLab        *Lab      `json:"media_lab,omitempty"`
	DisplayRGB      []float64 `json:"display_rgb,omitempty"`
	DisplayCodes    []int     `json:"display_codes,omitempty"`

	DeltaE         *float64 `json:"delta_e"`
	DeltaE00       *float64 `json:"delta_e00"`
	DeltaE76       *float64 `json:"delta_e76,omitempty"`
	DeltaEReliable *bool    `json:"delta_e_reliable,omitempty"`

	Boundary BoundaryReport `json:"boundary"`
	Error    string         `json:"error,omitempty"`
}

// DeltaEStats summarizes per-sample differences for one intent.
type DeltaEStats struct {
	Count          int     `json:"count"`
	Min            float64 `json:"min"`
	Mean           float64 `json:"mean"`
	Max            float64 `json:"max"`
	MaxSampleIndex int     `json:"max_sample_index,omitempty"`
}

// IntentSummary contains boundary counts and color-difference statistics.
type IntentSummary struct {
	Intent                Intent      `json:"intent"`
	IntentICCValue        uint32      `json:"intent_icc_value"`
	SampleCount           int         `json:"sample_count"`
	SourceUnrepresentable int         `json:"source_unrepresentable_count"`
	MediaOutOfGamut       int         `json:"media_out_of_gamut_count"`
	DeviceClipped         int         `json:"device_clipped_count"`
	GamutAlarm            int         `json:"gamut_alarm_count"`
	UnreliableDeltaE      int         `json:"unreliable_delta_e_count,omitempty"`
	DeltaE                DeltaEStats `json:"delta_e"`
}

// Report is the stable JSON/CSV report.
type Report struct {
	SchemaVersion     string             `json:"schema_version"`
	GeneratedAt       time.Time          `json:"generated_at"`
	Intents           []Intent           `json:"intents"`
	SourceProfile     ProfileInfo        `json:"source_profile"`
	MediaProfile      ProfileInfo        `json:"media_profile"`
	DisplayProfile    *ProfileInfo       `json:"display_profile,omitempty"`
	Config            ProofConfig        `json:"config"`
	Summaries         []IntentSummary    `json:"summaries"`
	IntentComparisons []IntentComparison `json:"intent_comparisons,omitempty"`
	Results           []SampleResult     `json:"results"`
}
