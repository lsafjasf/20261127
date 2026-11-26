// Package cadence detects field cadence in interlaced video.
//
// The analyzer works on decoded field pairs. It deliberately does not use the
// container's "interlaced" flag as a cadence decision: both 3:2 pulldown and
// true 30p can be carried in an interlaced stream.
package cadence

import (
	"errors"
	"fmt"
)

// Parity is the spatial parity of a field.
type Parity string

const (
	Top    Parity = "top"
	Bottom Parity = "bottom"
)

// FieldOrder is the temporal order of the two fields in one coded frame.
type FieldOrder string

const (
	OrderTFF     FieldOrder = "tff"
	OrderBFF     FieldOrder = "bff"
	OrderUnknown FieldOrder = "unknown"
)

// Cadence is the detected source cadence.
type Cadence string

const (
	Cadence24p32   Cadence = "24p_3:2"
	Cadence30p22   Cadence = "30p_2:2"
	CadenceUnknown Cadence = "unknown"
)

// Image is a single-channel field image in raster order.
type Image struct {
	Width  int
	Height int
	Pix    []uint8
}

// Validate reports malformed image data before it reaches the analyzer.
func (im Image) Validate() error {
	if im.Width <= 0 || im.Height <= 0 {
		return fmt.Errorf("invalid image size %dx%d", im.Width, im.Height)
	}
	if len(im.Pix) != im.Width*im.Height {
		return fmt.Errorf("image buffer has %d bytes, want %d", len(im.Pix), im.Width*im.Height)
	}
	return nil
}

// Frame is one coded frame split into its top and bottom spatial fields.
type Frame struct {
	Top    Image
	Bottom Image
}

// Media describes the field-pair sequence supplied to Analyze.
type Media struct {
	Frames     []Frame
	FrameRate  float64
	FieldOrder FieldOrderHint
}

// FieldOrderHint is metadata supplied by the container. It is only a prior;
// the analyzer still tests both temporal orders against the field content.
type FieldOrderHint string

const (
	HintTFF         FieldOrderHint = "tff"
	HintBFF         FieldOrderHint = "bff"
	HintProgressive FieldOrderHint = "progressive"
	HintUnknown     FieldOrderHint = "unknown"
)

// Options controls local windowing and segmentation.
type Options struct {
	// WindowFrames is the evidence window used for each local decision.
	// It is clamped to a usable range by the analyzer.
	WindowFrames int
	// MinSegmentFrames prevents a one-off bad window from creating a segment.
	MinSegmentFrames int
	// ChangePenalty is the cost of changing cadence state. Higher values
	// produce longer, more stable segments.
	ChangePenalty float64
	// SceneCutThreshold is a normalized field-distance threshold. Zero uses
	// the analyzer default.
	SceneCutThreshold float64
}

// DefaultOptions returns conservative defaults for broadcast-rate material.
func DefaultOptions() Options {
	return Options{
		WindowFrames:      16,
		MinSegmentFrames:  4,
		ChangePenalty:     0.32,
		SceneCutThreshold: 0.24,
	}
}

// Report is the complete, per-segment result.
type Report struct {
	Version        int            `json:"version"`
	FrameRate      float64        `json:"frame_rate"`
	FieldRate      float64        `json:"field_rate"`
	FrameCount     int            `json:"frame_count"`
	FieldCount     int            `json:"field_count"`
	DurationSec    float64        `json:"duration_sec"`
	ContainerOrder FieldOrderHint `json:"container_field_order,omitempty"`
	Segments       []Segment      `json:"segments"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// Segment is an independently classified span. StartFrame is inclusive and
// EndFrame is exclusive. Field ranges follow the same convention.
type Segment struct {
	Index       int        `json:"index"`
	StartFrame  int        `json:"start_frame"`
	EndFrame    int        `json:"end_frame"`
	StartField  int        `json:"start_field"`
	EndField    int        `json:"end_field"`
	StartSec    float64    `json:"start_sec"`
	EndSec      float64    `json:"end_sec"`
	Cadence     Cadence    `json:"cadence"`
	FieldOrder  FieldOrder `json:"field_order"`
	Confidence  float64    `json:"confidence"`
	Evidence    Evidence   `json:"evidence"`
	Recommended string     `json:"recommended_action"`
}

// Evidence contains the measurements behind a segment decision. Scores are
// normalized to [0,1] unless explicitly named as distances.
type Evidence struct {
	Basis              []string `json:"basis"`
	RepeatObserved     int      `json:"repeat_observed"`
	RepeatExpected     int      `json:"repeat_expected"`
	RepeatConsistency  float64  `json:"repeat_consistency"`
	RepeatPhase        int      `json:"repeat_phase"`
	PeriodFields       int      `json:"period_fields"`
	PeriodFit          float64  `json:"period_fit"`
	MixedFieldPairs    int      `json:"mixed_field_pairs"`
	CoherentFieldPairs int      `json:"coherent_field_pairs"`
	CoherentPairRatio  float64  `json:"coherent_pair_ratio"`
	WithinContrast     float64  `json:"within_pair_contrast"`
	MotionContinuity   float64  `json:"motion_continuity"`
	FieldOrderScore    float64  `json:"field_order_score"`
	FieldOrderSource   string   `json:"field_order_source"`
	WithinPairMotion   float64  `json:"within_pair_motion"`
	AcrossPairMotion   float64  `json:"across_pair_motion"`
	MedianFieldMotion  float64  `json:"median_field_motion"`
	SceneCuts          int      `json:"scene_cuts"`
	StaticRatio        float64  `json:"static_ratio"`
	AnalyzedFields     int      `json:"analyzed_fields"`
}

// AnalysisError identifies invalid input rather than an ambiguous cadence.
type AnalysisError struct {
	Err error
}

func (e *AnalysisError) Error() string { return "cadence: " + e.Err.Error() }
func (e *AnalysisError) Unwrap() error { return e.Err }

func analysisError(format string, args ...any) error {
	return &AnalysisError{Err: fmt.Errorf(format, args...)}
}

var (
	// ErrNoVideo is returned when there are no coded frames to inspect.
	ErrNoVideo = errors.New("no video frames")
	// ErrShortVideo means the sample cannot support a cadence conclusion.
	ErrShortVideo = errors.New("video is too short for cadence analysis")
)

func round3(v float64) float64 {
	return float64(int64(v*1000+0.5)) / 1000
}

func durationSec(frames int, frameRate float64) float64 {
	if frameRate <= 0 {
		return 0
	}
	return float64(frames) / frameRate
}
