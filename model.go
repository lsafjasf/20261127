package tearcheck

import "time"

// Cause is the primary diagnosis for one candidate event.
type Cause string

const (
	CauseHardwareSync Cause = "hardware_sync"
	CauseEncoding     Cause = "encoding_error"
	CauseContent      Cause = "content_effect"
	CauseNormal       Cause = "normal"
	CauseMixed        Cause = "mixed"
	CauseManual       Cause = "manual_review"
)

// Confidence describes whether a cause is safe to automate.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Side identifies which part of an intra-frame split repeats an adjacent frame.
type Side string

const (
	SideNone  Side = "none"
	SideAbove Side = "above"
	SideBelow Side = "below"
	SideBoth  Side = "both"
)

// Observation is the normalized evidence used by the deterministic classifier.
// Every confidence-like field is in [0,1]. Zero means "no evidence", not
// "measured and proved absent".
type Observation struct {
	FrameIndex int     `json:"frame_index"`
	Time       float64 `json:"time_seconds,omitempty"`
	SeamY      int     `json:"seam_y,omitempty"`

	// IntraFrameTear is the strength and width support of a horizontal fault.
	IntraFrameTear float64 `json:"intra_frame_tear"`
	SeamCoverage   float64 `json:"seam_coverage,omitempty"`

	// AdjacentFrameMatch is high when one side of the seam matches the previous
	// or next frame while the other side changes.
	AdjacentFrameMatch float64 `json:"adjacent_frame_match,omitempty"`
	MatchContrast      float64 `json:"match_contrast,omitempty"`
	RepeatedSide       Side    `json:"repeated_side,omitempty"`
	HorizontalShift    int     `json:"horizontal_shift,omitempty"`

	// TimingFault combines irregular capture intervals and capture/PTS drift.
	TimingFault float64 `json:"timing_fault,omitempty"`

	// ArbitraryScanline is high for a full scanline that is not explained by
	// the codec block grid.
	ArbitraryScanline float64 `json:"arbitrary_scanline,omitempty"`

	// BlockAlignment is high when the fault is on a plausible codec boundary.
	BlockAlignment      float64 `json:"block_alignment,omitempty"`
	BlockArtifact       float64 `json:"block_artifact,omitempty"`
	TemporalBlockRepeat float64 `json:"temporal_block_repeat,omitempty"`
	CodecError          float64 `json:"codec_error,omitempty"`

	// ContentMarker may come from game logs, scene tags, or a trusted effect mask.
	ContentMarker   float64 `json:"content_marker,omitempty"`
	ChromaEffect    float64 `json:"chroma_effect,omitempty"`
	EffectCoherence float64 `json:"effect_coherence,omitempty"`

	// Messages are human-readable facts retained for review and JSON output.
	Messages []string `json:"messages,omitempty"`
}

// Evidence is one weighted signal and its measured value.
type Evidence struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Value  float64 `json:"value"`
	Detail string  `json:"detail,omitempty"`
}

// ManualItem is retained when evidence is conflicting or too weak.
type ManualItem struct {
	Reason         string            `json:"reason"`
	PossibleCauses []Cause           `json:"possible_causes"`
	Scores         map[Cause]float64 `json:"scores"`
	Observation    Observation       `json:"observation"`
}

// Finding is a classified candidate event.
type Finding struct {
	ID             string            `json:"id"`
	Cause          Cause             `json:"cause"`
	Confidence     Confidence        `json:"confidence"`
	Score          float64           `json:"score"`
	Margin         float64           `json:"margin"`
	Scores         map[Cause]float64 `json:"scores"`
	Evidence       []Evidence        `json:"evidence"`
	FrameIndex     int               `json:"frame_index"`
	Time           float64           `json:"time_seconds,omitempty"`
	SeamY          int               `json:"seam_y,omitempty"`
	ReviewRequired bool              `json:"review_required"`
	Manual         *ManualItem       `json:"manual,omitempty"`
}

// Summary gives aggregate counts without discarding low-confidence items.
type Summary struct {
	FramesAnalyzed int           `json:"frames_analyzed"`
	Findings       int           `json:"findings"`
	Overall        Cause         `json:"overall"`
	ByCause        map[Cause]int `json:"by_cause"`
	ManualReview   int           `json:"manual_review"`
}

// Report is the stable JSON result returned by the package and CLI.
type Report struct {
	SchemaVersion string    `json:"schema_version"`
	Source        string    `json:"source,omitempty"`
	Width         int       `json:"width,omitempty"`
	Height        int       `json:"height,omitempty"`
	Codec         string    `json:"codec,omitempty"`
	GeneratedAt   time.Time `json:"generated_at"`
	Summary       Summary   `json:"summary"`
	Findings      []Finding `json:"findings"`
	ManualReviews []Finding `json:"manual_reviews"`
	Warnings      []string  `json:"warnings,omitempty"`
}

// Metadata is optional side-car information that cannot be inferred from pixels.
type Metadata struct {
	Source               string         `json:"source,omitempty"`
	Codec                string         `json:"codec,omitempty"`
	Width                int            `json:"width,omitempty"`
	Height               int            `json:"height,omitempty"`
	FrameTiming          []FrameTiming  `json:"frame_timing,omitempty"`
	ContentEffectWindows []EffectWindow `json:"content_effect_windows,omitempty"`
	CodecEvents          []CodecEvent   `json:"codec_events,omitempty"`
}

// FrameTiming contains capture and presentation timestamps in seconds.
type FrameTiming struct {
	FrameIndex       int     `json:"frame_index"`
	CaptureSeconds   float64 `json:"capture_seconds,omitempty"`
	PTSSeconds       float64 `json:"pts_seconds,omitempty"`
	ExpectedInterval float64 `json:"expected_interval,omitempty"`
}

// EffectWindow marks a trusted game/content effect interval.
type EffectWindow struct {
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
	Confidence   float64 `json:"confidence"`
	Label        string  `json:"label,omitempty"`
}

// CodecEvent is a bitstream/decoder error or concealment event.
type CodecEvent struct {
	FrameIndex int     `json:"frame_index,omitempty"`
	Time       float64 `json:"time_seconds,omitempty"`
	Y          int     `json:"y,omitempty"`
	Confidence float64 `json:"confidence"`
	Message    string  `json:"message,omitempty"`
}
