package align

import (
	"fmt"
	"math"
)

var defaultConfig = Config{
	MinAnchorConfidence:     0.55,
	MinOffsetAnchors:        2,
	MinDriftAnchors:         3,
	MinDriftBaselineSec:     60,
	MaxDriftPPM:             1500,
	AnchorToleranceMS:       25,
	BaseUncertaintyMS:       8,
	MaxUnverifiedDriftPPM:   500,
	ConfirmedDriftPPM:       40,
	MaxAnchorGapSec:         90,
	MaxExtrapolationSec:     60,
	MaxMappingErrorMS:       20,
	PropagationFactor:       0.85,
	AmbientConfidenceFactor: 0.9,
}

// Normalize applies defaults and validates numeric ranges.
func (c Config) Normalize() (Config, error) {
	c = fillDefaults(c, defaultConfig)
	checks := []struct {
		name     string
		value    float64
		min, max float64
	}{
		{"min_anchor_confidence", c.MinAnchorConfidence, 0, 1},
		{"max_drift_ppm", c.MaxDriftPPM, 0, 100000},
		{"anchor_tolerance_ms", c.AnchorToleranceMS, 0.1, 10000},
		{"base_uncertainty_ms", c.BaseUncertaintyMS, 0, 10000},
		{"max_unverified_drift_ppm", c.MaxUnverifiedDriftPPM, 0, 100000},
		{"confirmed_drift_uncertainty_ppm", c.ConfirmedDriftPPM, 0.1, 100000},
		{"max_anchor_gap_sec", c.MaxAnchorGapSec, 0, 1e9},
		{"max_extrapolation_sec", c.MaxExtrapolationSec, 0, 1e9},
		{"max_mapping_error_ms", c.MaxMappingErrorMS, 0.1, 10000},
		{"propagation_factor", c.PropagationFactor, 0, 1},
		{"ambient_confidence_factor", c.AmbientConfidenceFactor, 0, 1},
		{"min_drift_baseline_sec", c.MinDriftBaselineSec, 0, 1e9},
	}
	for _, check := range checks {
		if math.IsNaN(check.value) || math.IsInf(check.value, 0) ||
			check.value < check.min || check.value > check.max {
			return Config{}, fmt.Errorf("%s must be in [%g,%g]", check.name, check.min, check.max)
		}
	}
	if c.MinOffsetAnchors < 1 || c.MinOffsetAnchors > 20 {
		return Config{}, fmt.Errorf("min_offset_anchors must be in [1,20]")
	}
	if c.MinDriftAnchors < 2 || c.MinDriftAnchors > 50 {
		return Config{}, fmt.Errorf("min_drift_anchors must be in [2,50]")
	}
	if c.MinDriftAnchors < c.MinOffsetAnchors {
		return Config{}, fmt.Errorf("min_drift_anchors must be >= min_offset_anchors")
	}
	return c, nil
}

func fillDefaults(value, fallback Config) Config {
	if value.MinAnchorConfidence == 0 {
		value.MinAnchorConfidence = fallback.MinAnchorConfidence
	}
	if value.MinOffsetAnchors == 0 {
		value.MinOffsetAnchors = fallback.MinOffsetAnchors
	}
	if value.MinDriftAnchors == 0 {
		value.MinDriftAnchors = fallback.MinDriftAnchors
	}
	if value.MinDriftBaselineSec == 0 {
		value.MinDriftBaselineSec = fallback.MinDriftBaselineSec
	}
	if value.MaxDriftPPM == 0 {
		value.MaxDriftPPM = fallback.MaxDriftPPM
	}
	if value.AnchorToleranceMS == 0 {
		value.AnchorToleranceMS = fallback.AnchorToleranceMS
	}
	if value.BaseUncertaintyMS == 0 {
		value.BaseUncertaintyMS = fallback.BaseUncertaintyMS
	}
	if value.MaxUnverifiedDriftPPM == 0 {
		value.MaxUnverifiedDriftPPM = fallback.MaxUnverifiedDriftPPM
	}
	if value.ConfirmedDriftPPM == 0 {
		value.ConfirmedDriftPPM = fallback.ConfirmedDriftPPM
	}
	if value.MaxAnchorGapSec == 0 {
		value.MaxAnchorGapSec = fallback.MaxAnchorGapSec
	}
	if value.MaxExtrapolationSec == 0 {
		value.MaxExtrapolationSec = fallback.MaxExtrapolationSec
	}
	if value.MaxMappingErrorMS == 0 {
		value.MaxMappingErrorMS = fallback.MaxMappingErrorMS
	}
	if value.PropagationFactor == 0 {
		value.PropagationFactor = fallback.PropagationFactor
	}
	if value.AmbientConfidenceFactor == 0 {
		value.AmbientConfidenceFactor = fallback.AmbientConfidenceFactor
	}
	return value
}
