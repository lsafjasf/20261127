package analyzer

// Rect is an axis-aligned region in source-frame pixel coordinates.
type Rect struct {
	X int
	Y int
	W int
	H int
}

// Options controls a before/after comparison.
type Options struct {
	SourcePaths []string
	ScaledPaths []string
	ROI         *Rect
	BlockSizes  []int
}

// GradientStats describes local image gradients in source-pixel units.
type GradientStats struct {
	Mean float64 `json:"mean"`
	RMS  float64 `json:"rms"`
	P50  float64 `json:"p50"`
	P90  float64 `json:"p90"`
	P99  float64 `json:"p99"`
}

// SpatialStats summarizes edge sharpness and high-frequency detail.
type SpatialStats struct {
	Gradient        GradientStats `json:"gradient"`
	LaplacianRMS    float64       `json:"laplacian_rms"`
	HFNorm          float64       `json:"hf_norm"`
	EdgeWidthMedian float64       `json:"edge_width_median_px"`
	EdgeWidthP75    float64       `json:"edge_width_p75_px"`
	EdgeSamples     int           `json:"edge_samples"`
}

// BlockSizeStats describes one candidate codec block pitch.
type BlockSizeStats struct {
	Size       int     `json:"size"`
	Axis       string  `json:"axis"`
	Phase      int     `json:"phase"`
	Ratio      float64 `json:"ratio"`
	Excess     float64 `json:"excess"`
	ZScore     float64 `json:"z_score"`
	Boundary   float64 `json:"boundary_diff"`
	Interior   float64 `json:"interior_diff"`
	Confidence float64 `json:"confidence"`
}

// BlockStats contains the strongest periodic boundary result.
type BlockStats struct {
	Best       BlockSizeStats   `json:"best"`
	Candidates []BlockSizeStats `json:"candidates"`
}

// TemporalStats describes frame-to-frame stability.
type TemporalStats struct {
	Frames        int     `json:"frames"`
	MeanAbsDelta  float64 `json:"mean_abs_delta"`
	DeltaNorm     float64 `json:"delta_norm"`
	EdgeNoise     float64 `json:"edge_noise"`
	EdgeNoiseNorm float64 `json:"edge_noise_norm"`
}

// SequenceStats is the complete analysis of one frame sequence.
type SequenceStats struct {
	Frames   int           `json:"frames"`
	Width    int           `json:"width"`
	Height   int           `json:"height"`
	Spatial  SpatialStats  `json:"spatial"`
	Block    BlockStats    `json:"block"`
	Temporal TemporalStats `json:"temporal"`
}

// Comparison contains scale-normalized before/after ratios.
type Comparison struct {
	ScaleX             float64 `json:"scale_x"`
	ScaleY             float64 `json:"scale_y"`
	Scale              float64 `json:"scale"`
	SharpnessRetention float64 `json:"sharpness_retention"`
	GradientRetention  float64 `json:"gradient_retention"`
	EdgeWidthRatio     float64 `json:"edge_width_ratio"`
	BlockExcessDelta   float64 `json:"block_excess_delta"`
	TemporalDeltaDiff  float64 `json:"temporal_delta_diff"`
	EdgeNoiseDelta     float64 `json:"edge_noise_delta"`
}

// Evidence is a human-readable finding used by the verdict.
type Evidence struct {
	Code  string  `json:"code"`
	Value float64 `json:"value,omitempty"`
	Text  string  `json:"text"`
}

// Verdict names the most likely origin of the observed softness.
type Verdict string

const (
	VerdictSource          Verdict = "source"
	VerdictEncoding        Verdict = "encoding"
	VerdictDisplaySampling Verdict = "display_sampling"
	VerdictSourceDisplay   Verdict = "source_and_display"
	VerdictInconclusive    Verdict = "inconclusive"
)

// Report is the stable output contract for both text and JSON modes.
type Report struct {
	Verdict    Verdict       `json:"verdict"`
	Confidence float64       `json:"confidence"`
	Summary    string        `json:"summary"`
	Source     SequenceStats `json:"source"`
	Scaled     SequenceStats `json:"scaled"`
	Comparison Comparison    `json:"comparison"`
	Evidence   []Evidence    `json:"evidence"`
}

type plane struct {
	W int
	H int
	P []float32
}

func (p *plane) at(x, y int) float64 {
	return float64(p.P[y*p.W+x])
}
