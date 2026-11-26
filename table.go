package flatfield

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const SchemaVersion = "flatfield.correction/v1"

// Kind selects one decomposed response for the primary Response and Gain
// fields of a Table.
type Kind string

const (
	KindObserved         Kind = "observed"
	KindDevice           Kind = "device"
	KindDeviceNoGradient Kind = "device_no_gradient"
	KindPanel            Kind = "panel"
	KindPanelResidual    Kind = "panel_residual"
	KindLens             Kind = "lens"
	KindGradient         Kind = "gradient"
)

// Reference selects the normalization anchor.
type Reference string

const (
	ReferenceCenter        Reference = "center"
	ReferenceGeometricMean Reference = "geometric_mean"
	ReferenceMaximum       Reference = "maximum"
)

// Options controls flat-field estimation.
type Options struct {
	GridWidth      int
	GridHeight     int
	OpticalCenterX float64
	OpticalCenterY float64
	AspectRatio    float64
	SmoothSigma    float64
	Reference      Reference
	Kind           Kind
	MaxGain        float64
	Lens           *Table
	MinGradientLog float64
}

// DefaultOptions returns deterministic defaults suitable for most images.
func DefaultOptions() Options {
	return Options{
		GridWidth:      32,
		GridHeight:     0,
		OpticalCenterX: 0.5,
		OpticalCenterY: 0.5,
		SmoothSigma:    0.8,
		Reference:      ReferenceCenter,
		Kind:           KindDevice,
		MaxGain:        16,
		MinGradientLog: 0.01,
	}
}

func (o Options) withDefaults(width, height int) (Options, error) {
	d := DefaultOptions()
	if o.GridWidth == 0 {
		o.GridWidth = d.GridWidth
	}
	if o.GridHeight == 0 {
		if o.AspectRatio > 0 {
			o.GridHeight = int(math.Round(float64(o.GridWidth) / o.AspectRatio))
		} else {
			o.GridHeight = int(math.Round(float64(o.GridWidth) * float64(height) / float64(width)))
		}
	}
	if o.OpticalCenterX == 0 && o.OpticalCenterY == 0 {
		o.OpticalCenterX, o.OpticalCenterY = 0.5, 0.5
	}
	if o.AspectRatio == 0 {
		o.AspectRatio = float64(width) / float64(height)
	}
	if o.SmoothSigma < 0 {
		o.SmoothSigma = 0
	} else if o.SmoothSigma == 0 {
		o.SmoothSigma = d.SmoothSigma
	}
	if o.Reference == "" {
		o.Reference = d.Reference
	}
	if o.Kind == "" {
		o.Kind = d.Kind
	}
	if o.MaxGain == 0 {
		o.MaxGain = d.MaxGain
	}
	if o.MinGradientLog == 0 {
		o.MinGradientLog = d.MinGradientLog
	}
	if o.GridWidth < 2 || o.GridHeight < 2 {
		return o, errors.New("flatfield: grid must be at least 2x2")
	}
	if o.GridWidth > width {
		o.GridWidth = width
	}
	if o.GridHeight > height {
		o.GridHeight = height
	}
	if o.GridWidth < 2 || o.GridHeight < 2 {
		return o, errors.New("flatfield: image is too small for a 2x2 grid")
	}
	if o.OpticalCenterX < 0 || o.OpticalCenterX > 1 || o.OpticalCenterY < 0 || o.OpticalCenterY > 1 {
		return o, errors.New("flatfield: optical center must use normalized [0,1] coordinates")
	}
	if o.AspectRatio <= 0 {
		return o, errors.New("flatfield: aspect ratio must be positive")
	}
	if o.MaxGain < -1 {
		return o, errors.New("flatfield: MaxGain must be non-negative or -1 for unlimited")
	}
	switch o.Reference {
	case ReferenceCenter, ReferenceGeometricMean, ReferenceMaximum:
	default:
		return o, fmt.Errorf("flatfield: unsupported reference %q", o.Reference)
	}
	switch o.Kind {
	case KindObserved, KindDevice, KindDeviceNoGradient, KindPanel, KindPanelResidual, KindLens, KindGradient:
	default:
		return o, fmt.Errorf("flatfield: unsupported table kind %q", o.Kind)
	}
	return o, nil
}

// CornerStats contains one corner measurement in raster order TL, TR, BR, BL.
type CornerStats struct {
	U        float64 `json:"u"`
	V        float64 `json:"v"`
	Response float64 `json:"response"`
	Gain     float64 `json:"gain"`
}

// ComponentStats summarizes one decomposed component.
type ComponentStats struct {
	LogRMS         float64       `json:"log_rms"`
	RelativeRMS    float64       `json:"relative_rms"`
	MinResponse    float64       `json:"min_response"`
	MaxResponse    float64       `json:"max_response"`
	CornerResponse []CornerStats `json:"corner_response"`
}

// GradientStats describes an affine smooth gradient.
type GradientStats struct {
	ComponentStats
	LogSlopeX          float64 `json:"log_slope_x"`
	LogSlopeY          float64 `json:"log_slope_y"`
	MagnitudePerHeight float64 `json:"magnitude_per_height"`
	AngleDegrees       float64 `json:"angle_degrees"`
	RSquared           float64 `json:"r_squared"`
	Significant        bool    `json:"significant"`
	Interpretation     string  `json:"interpretation"`
}

// LensStats describes the fitted radial lens component.
type LensStats struct {
	ComponentStats
	Coefficients [3]float64 `json:"log_r2_r4_r6"`
	Model        string     `json:"model"`
	Reference    string     `json:"reference,omitempty"`
}

// Report contains decomposition diagnostics.
type Report struct {
	SourceWidth     int                `json:"source_width"`
	SourceHeight    int                `json:"source_height"`
	GridWidth       int                `json:"grid_width"`
	GridHeight      int                `json:"grid_height"`
	MeanLinearLevel float64            `json:"mean_linear_level"`
	Observed        ComponentStats     `json:"observed"`
	Gradient        GradientStats      `json:"gradient"`
	Lens            LensStats          `json:"lens"`
	Panel           ComponentStats     `json:"panel"`
	PanelResidual   ComponentStats     `json:"panel_residual"`
	DominantCause   string             `json:"dominant_cause"`
	Identifiability string             `json:"identifiability"`
	Confidence      float64            `json:"confidence"`
	Contributions   map[string]float64 `json:"cause_contributions,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
}

// Table is a resolution-independent correction table.
type Table struct {
	Schema           string    `json:"schema"`
	Kind             Kind      `json:"kind"`
	GridWidth        int       `json:"grid_width"`
	GridHeight       int       `json:"grid_height"`
	CoordinateSystem string    `json:"coordinate_system"`
	ToneDomain       string    `json:"tone_domain"`
	OpticalCenterX   float64   `json:"optical_center_x"`
	OpticalCenterY   float64   `json:"optical_center_y"`
	AspectRatio      float64   `json:"aspect_ratio"`
	Reference        Reference `json:"reference"`
	MaxGain          float64   `json:"max_gain,omitempty"`

	// Response and Gain correspond to Kind. Arrays are row-major.
	Response []float64 `json:"response"`
	Gain     []float64 `json:"gain"`

	ObservedResponse []float64 `json:"observed_response"`
	GradientResponse []float64 `json:"gradient_response"`
	LensResponse     []float64 `json:"lens_response"`
	PanelResponse    []float64 `json:"panel_response"`
	// PanelResidualResponse is PanelResponse divided by GradientResponse.
	PanelResidualResponse []float64 `json:"panel_residual_response,omitempty"`

	Report Report `json:"report"`
}

// SaveJSON writes the table as indented JSON.
func (t *Table) SaveJSON(path string) error {
	if t == nil {
		return errors.New("flatfield: nil table")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && path != filepath.Base(path) {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// LoadTable reads a JSON correction table.
func LoadTable(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Table
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("flatfield: parse table %s: %w", path, err)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// Validate checks table shape and finite values.
func (t *Table) Validate() error {
	if t == nil {
		return errors.New("flatfield: nil table")
	}
	if t.GridWidth < 2 || t.GridHeight < 2 {
		return errors.New("flatfield: table grid must be at least 2x2")
	}
	n := t.GridWidth * t.GridHeight
	required := map[string][]float64{
		"response": t.Response,
		"gain":     t.Gain,
	}
	for name, a := range required {
		if len(a) != n {
			return fmt.Errorf("flatfield: table %s length %d, want %d", name, len(a), n)
		}
		for i, v := range a {
			if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
				return fmt.Errorf("flatfield: table %s[%d] is not positive and finite", name, i)
			}
		}
	}
	optional := map[string][]float64{
		"observed_response":       t.ObservedResponse,
		"gradient_response":       t.GradientResponse,
		"lens_response":           t.LensResponse,
		"panel_response":          t.PanelResponse,
		"panel_residual_response": t.PanelResidualResponse,
	}
	for name, a := range optional {
		if len(a) == 0 {
			continue
		}
		if len(a) != n {
			return fmt.Errorf("flatfield: table %s length %d, want %d", name, len(a), n)
		}
		for i, v := range a {
			if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
				return fmt.Errorf("flatfield: table %s[%d] is not positive and finite", name, i)
			}
		}
	}
	if t.OpticalCenterX < 0 || t.OpticalCenterX > 1 || t.OpticalCenterY < 0 || t.OpticalCenterY > 1 {
		return errors.New("flatfield: optical center must be normalized [0,1]")
	}
	return nil
}
