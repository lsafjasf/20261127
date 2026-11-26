// Package lighting separates periodic illumination changes from camera
// exposure changes and real, sustained changes in outdoor footage.
package lighting

import "fmt"

const Version = "1.0"

// Config controls frame normalization and evidence thresholds. Luma values
// and thresholds are expressed on the 0..255 scale of an 8-bit Y plane.
type Config struct {
	MaxAnalysisWidth    int
	MotionThreshold     float64
	StableActivityDecay float64
	EventSlopeThreshold float64
	EventMinAmplitude   float64
	SlowCutoffHz        float64
	ExposureBandHighHz  float64
	FlickerBandLowHz    float64
	FlickerMinRMS       float64
	FlickerMinRelative  float64
	FlickerMinPeak      float64
	RegionCols          int
	RegionRows          int
}

// DefaultConfig returns conservative defaults for normal outdoor video.
func DefaultConfig() Config {
	return Config{
		MaxAnalysisWidth:    320,
		MotionThreshold:     7,
		StableActivityDecay: 0.88,
		EventSlopeThreshold: 0.18,
		EventMinAmplitude:   2.5,
		SlowCutoffHz:        0.5,
		ExposureBandHighHz:  4,
		FlickerBandLowHz:    4,
		FlickerMinRMS:       0.9,
		FlickerMinRelative:  0.25,
		FlickerMinPeak:      3.5,
		RegionCols:          3,
		RegionRows:          3,
	}
}

func (c Config) validate() (Config, error) {
	if c.MaxAnalysisWidth < 16 {
		return c, fmt.Errorf("max analysis width must be at least 16")
	}
	if c.MotionThreshold <= 0 {
		return c, fmt.Errorf("motion threshold must be positive")
	}
	if c.StableActivityDecay <= 0 || c.StableActivityDecay >= 1 {
		return c, fmt.Errorf("stable activity decay must be in (0,1)")
	}
	if c.EventSlopeThreshold <= 0 || c.EventMinAmplitude <= 0 {
		return c, fmt.Errorf("event thresholds must be positive")
	}
	if c.SlowCutoffHz <= 0 || c.ExposureBandHighHz <= c.SlowCutoffHz {
		return c, fmt.Errorf("frequency bands must be ordered")
	}
	if c.FlickerBandLowHz < c.ExposureBandHighHz {
		return c, fmt.Errorf("flicker band must start at or above exposure band")
	}
	if c.FlickerMinRMS <= 0 || c.FlickerMinRelative <= 0 || c.FlickerMinPeak <= 0 {
		return c, fmt.Errorf("flicker thresholds must be positive")
	}
	if c.RegionCols < 1 || c.RegionRows < 1 {
		return c, fmt.Errorf("region grid must be positive")
	}
	return c, nil
}

// Frame is one luminance frame. Timestamp is in seconds from the clip start.
type Frame struct {
	Luma      []byte
	Width     int
	Height    int
	Timestamp float64
}

// Report is the stable JSON-facing result of an analysis.
type Report struct {
	Version        string          `json:"version"`
	Source         Source          `json:"source"`
	Config         EffectiveConfig `json:"config"`
	Classification Classification  `json:"classification"`
	Evidence       Evidence        `json:"evidence"`
	Bands          Bands           `json:"bands"`
	Events         []Event         `json:"events"`
	Curves         Curves          `json:"curves"`
	Warnings       []string        `json:"warnings,omitempty"`
}

type Source struct {
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        float64 `json:"fps"`
	FrameCount int     `json:"frame_count"`
	DurationS  float64 `json:"duration_s"`
}

type EffectiveConfig struct {
	MaxAnalysisWidth    int     `json:"max_analysis_width"`
	MotionThreshold     float64 `json:"motion_threshold_luma"`
	SlowCutoffHz        float64 `json:"slow_cutoff_hz"`
	ExposureBandHighHz  float64 `json:"exposure_band_high_hz"`
	FlickerBandLowHz    float64 `json:"flicker_band_low_hz"`
	FlickerMinRMS       float64 `json:"flicker_min_rms_luma"`
	EventSlopeThreshold float64 `json:"event_slope_threshold_luma_s"`
	EventMinAmplitude   float64 `json:"event_min_amplitude_luma"`
}

type Classification struct {
	Primary    string   `json:"primary"`
	Confidence float64  `json:"confidence"`
	Reasons    []string `json:"reasons"`
}

// Evidence keeps the three independent observation channels separate.
type Evidence struct {
	StableBackground BackgroundEvidence `json:"stable_background"`
	ForegroundMotion MotionEvidence     `json:"foreground_motion"`
	GlobalBrightness GlobalEvidence     `json:"global_brightness"`
	Flicker          FlickerEvidence    `json:"flicker"`
}

type BackgroundEvidence struct {
	MeanFraction   float64 `json:"mean_fraction"`
	MinFraction    float64 `json:"min_fraction"`
	SlowRMS        float64 `json:"slow_rms_luma"`
	SustainedRange float64 `json:"sustained_range_luma"`
	Correlation    float64 `json:"correlation_with_global"`
}

type MotionEvidence struct {
	MeanFraction      float64 `json:"mean_fraction"`
	PeakFraction      float64 `json:"peak_fraction"`
	MeanEnergy        float64 `json:"mean_residual_luma"`
	FlickerRMS        float64 `json:"flicker_rms_luma"`
	GlobalCorrelation float64 `json:"correlation_with_global"`
	FlickerIndep      float64 `json:"flicker_independence"`
}

type GlobalEvidence struct {
	Range              float64 `json:"range_luma"`
	SustainedRange     float64 `json:"sustained_range_luma"`
	SlowRMS            float64 `json:"slow_rms_luma"`
	FlickerRMS         float64 `json:"flicker_rms_luma"`
	BackgroundCorr     float64 `json:"correlation_with_background"`
	RegionCoherence    float64 `json:"region_coherence"`
	DominantChangeRate float64 `json:"dominant_change_rate_luma_s"`
}

type FlickerEvidence struct {
	BackgroundRMS        float64 `json:"background_rms_luma"`
	BackgroundRelative   float64 `json:"background_flicker_relative_power"`
	PeakHz               float64 `json:"peak_hz"`
	PeakConcentration    float64 `json:"peak_concentration"`
	MotionBandRatio      float64 `json:"motion_to_background_band_ratio"`
	Periodic             bool    `json:"periodic"`
	AutoExposureConfound bool    `json:"exposure_confound"`
}

type BandStat struct {
	LowHz             float64 `json:"low_hz"`
	HighHz            float64 `json:"high_hz"`
	RMSLuma           float64 `json:"rms_luma"`
	RelativePower     float64 `json:"relative_power"`
	PeakHz            float64 `json:"peak_hz"`
	PeakConcentration float64 `json:"peak_concentration"`
	PeakAtEdge        bool    `json:"peak_at_band_edge"`
}

type Bands struct {
	Global     []BandStat `json:"global"`
	Background []BandStat `json:"stable_background"`
	Motion     []BandStat `json:"foreground_motion"`
}

type EventEvidence struct {
	DurationS          float64 `json:"duration_s"`
	AmplitudeLuma      float64 `json:"signed_amplitude_luma"`
	PeakSlopeLumaS     float64 `json:"peak_slope_luma_s"`
	BackgroundFraction float64 `json:"mean_stable_background_fraction"`
	MotionFraction     float64 `json:"mean_motion_fraction"`
	RegionUniformity   float64 `json:"region_uniformity"`
	RegionCoverage     float64 `json:"region_coverage"`
	RegionPhaseSpreadS float64 `json:"region_phase_spread_s"`
	FlickerRelative    float64 `json:"background_flicker_relative_power"`
}

type Event struct {
	Kind       string        `json:"kind"`
	StartS     float64       `json:"start_s"`
	EndS       float64       `json:"end_s"`
	Amplitude  float64       `json:"amplitude_luma"`
	Frequency  float64       `json:"frequency_hz,omitempty"`
	Confidence float64       `json:"confidence"`
	Retained   bool          `json:"retained"`
	Evidence   EventEvidence `json:"evidence"`
}

// Curves contains one value per decoded frame. RegionLuma is a row-major
// RegionRows x RegionCols list, where every entry is a time series.
type Curves struct {
	TimeS                    []float64   `json:"time_s"`
	GlobalLuma               []float64   `json:"global_luma"`
	StableBackgroundLuma     []float64   `json:"stable_background_luma"`
	StableBackgroundFraction []float64   `json:"stable_background_fraction"`
	ForegroundLuma           []float64   `json:"foreground_luma"`
	ForegroundMotion         []float64   `json:"foreground_motion_fraction"`
	SustainedLuma            []float64   `json:"sustained_luma"`
	SustainedBackgroundLuma  []float64   `json:"sustained_background_luma"`
	FastResidualLuma         []float64   `json:"fast_residual_luma"`
	RegionLuma               [][]float64 `json:"stable_region_luma"`
}
