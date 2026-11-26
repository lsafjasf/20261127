// Package rangecheck performs frame-by-frame legal-range and spatial-source
// analysis for limited-range video deliverables.
package rangecheck

import "time"

// RangeKind selects the nominal legal code range.
type RangeKind string

const (
	// RangeTV is limited/ video range: Y 16..235 and C 16..240 at 8 bits.
	RangeTV RangeKind = "tv"
	// RangeFull is full range: 0..255. It is useful for metadata mismatch checks.
	RangeFull RangeKind = "full"
)

// PixelFormat describes the byte layout supplied to the analyzer.
type PixelFormat string

const (
	FormatYUV420P PixelFormat = "yuv420p"
	FormatYUV444P PixelFormat = "yuv444p"
	FormatGray8   PixelFormat = "gray8"
	FormatRGB24   PixelFormat = "rgb24"
	FormatRGBA32  PixelFormat = "rgba32"
)

// Settings controls analysis and heuristic thresholds.
type Settings struct {
	Range             RangeKind   `json:"range"`
	PixelFormat       PixelFormat `json:"pixel_format"`
	Width             int         `json:"width"`
	Height            int         `json:"height"`
	Bits              int         `json:"bits"`
	ColorMatrix       string      `json:"color_matrix"`
	FPS               Rational    `json:"fps"`
	MinRegionPixels   int         `json:"min_region_pixels"`
	ReportFramePixels bool        `json:"report_frame_pixels"`
	GenerateOverlays  bool        `json:"generate_overlays"`
	OverlayDir        string      `json:"overlay_dir,omitempty"`
	MaxFrameReports   int         `json:"max_frame_reports,omitempty"`
}

// Rational is an exact frame rate or time base.
type Rational struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

func (r Rational) Float64() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}

// Frame is one decoded source frame. Planes use raw code values. For packed
// RGB formats, RGB is populated and Y/Cb/Cr are derived for statistics.
type Frame struct {
	Index      int
	PTS        time.Duration
	PTSSeconds float64
	Width      int
	Height     int
	Format     PixelFormat
	Y          []byte
	Cb         []byte
	Cr         []byte
	RGB        []byte
	StrideY    int
	StrideC    int
}

// LevelBounds contains legal and soft-edge boundaries for one plane.
type LevelBounds struct {
	Black int `json:"black"`
	White int `json:"white"`
}

// PlaneStats contains exact code-value counts for one frame/plane.
type PlaneStats struct {
	Plane           string  `json:"plane"`
	Pixels          int     `json:"pixels"`
	Min             int     `json:"min"`
	Max             int     `json:"max"`
	Mean            float64 `json:"mean"`
	BelowLegal      int     `json:"below_legal"`
	WithinLegal     int     `json:"within_legal"`
	AboveLegal      int     `json:"above_legal"`
	BelowSoftEdge   int     `json:"below_soft_edge"`
	AboveSoftEdge   int     `json:"above_soft_edge"`
	ExactBlack      int     `json:"exact_black"`
	ExactWhite      int     `json:"exact_white"`
	NearBlackLegal  int     `json:"near_black_legal"`
	NearWhiteLegal  int     `json:"near_white_legal"`
	BelowRatio      float64 `json:"below_ratio"`
	AboveRatio      float64 `json:"above_ratio"`
	OutOfRangeRatio float64 `json:"out_of_range_ratio"`
}

// Rect is a pixel rectangle. Coordinates are inclusive of X/Y origin and use
// half-open width/height.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r Rect) Area() int { return r.Width * r.Height }

// RegionKind identifies which legal-range side a region belongs to.
type RegionKind string

const (
	KindSuperBlack RegionKind = "super_black"
	KindSuperWhite RegionKind = "super_white"
)

// SourceClass is the cautious provenance classification for a region.
type SourceClass string

const (
	SourceBurnedSubtitle SourceClass = "burned_subtitle_candidate"
	SourceGraphicText    SourceClass = "graphic_or_ui_text_candidate"
	SourceHighlight      SourceClass = "scene_highlight_candidate"
	SourceCodecEdge      SourceClass = "codec_edge_overshoot"
	SourceLevelShift     SourceClass = "codec_or_range_level_shift"
	SourceRangeMismatch  SourceClass = "global_range_mismatch"
	SourceStylizedGrade  SourceClass = "stylized_grade_candidate"
	SourceUnknown        SourceClass = "unknown_region"
)

// Verdict separates technical compliance from likely intent.
type Verdict string

const (
	VerdictEncodingError    Verdict = "encoding_or_mastering_error"
	VerdictIntentionalStyle Verdict = "intentional_style_but_check_delivery_range"
	VerdictAmbiguous        Verdict = "ambiguous_manual_review"
	VerdictLegal            Verdict = "legal"
)

// Region is a connected or grouped spatial source within one frame.
type Region struct {
	ID             int         `json:"id"`
	TrackID        int         `json:"track_id,omitempty"`
	Plane          string      `json:"plane"`
	Kind           RegionKind  `json:"kind"`
	Rect           Rect        `json:"rect"`
	PixelCount     int         `json:"pixel_count"`
	MinCode        int         `json:"min_code"`
	MaxCode        int         `json:"max_code"`
	MeanCode       float64     `json:"mean_code"`
	ContextMean    float64     `json:"context_mean"`
	LocalContrast  float64     `json:"local_contrast"`
	FillRatio      float64     `json:"fill_ratio"`
	BottomBand     bool        `json:"bottom_band"`
	EdgeLike       bool        `json:"edge_like"`
	Source         SourceClass `json:"source"`
	Verdict        Verdict     `json:"verdict"`
	Confidence     float64     `json:"confidence"`
	Reasons        []string    `json:"reasons"`
	ComponentCount int         `json:"component_count,omitempty"`
}

// FrameReport is the required frame-level evidence; callers should not infer
// a segment from whole-video peaks alone.
type FrameReport struct {
	Index            int         `json:"frame"`
	PTSSeconds       float64     `json:"pts_seconds"`
	Width            int         `json:"width"`
	Height           int         `json:"height"`
	HasOutOfRange    bool        `json:"has_out_of_range"`
	Y                PlaneStats  `json:"y"`
	Cb               *PlaneStats `json:"cb,omitempty"`
	Cr               *PlaneStats `json:"cr,omitempty"`
	OutOfRangePixels int         `json:"out_of_range_pixels"`
	OutOfRangeRatio  float64     `json:"out_of_range_ratio"`
	BlackPixels      int         `json:"super_black_pixels"`
	WhitePixels      int         `json:"super_white_pixels"`
	Regions          []Region    `json:"regions,omitempty"`
	OverlayPath      string      `json:"overlay_path,omitempty"`
}

// Track summarizes a region across frames.
type Track struct {
	ID             int         `json:"id"`
	Kind           RegionKind  `json:"kind"`
	Source         SourceClass `json:"source"`
	Verdict        Verdict     `json:"verdict"`
	FirstFrame     int         `json:"first_frame"`
	LastFrame      int         `json:"last_frame"`
	StartSeconds   float64     `json:"start_seconds"`
	EndSeconds     float64     `json:"end_seconds"`
	FrameCount     int         `json:"frame_count"`
	Representative Rect        `json:"representative_rect"`
	BestConfidence float64     `json:"best_confidence"`
	Reasons        []string    `json:"reasons"`
}

// Segment is a contiguous run of frames sharing an anomaly pattern.
type Segment struct {
	ID             int      `json:"id"`
	FirstFrame     int      `json:"first_frame"`
	LastFrame      int      `json:"last_frame"`
	StartSeconds   float64  `json:"start_seconds"`
	EndSeconds     float64  `json:"end_seconds"`
	FrameCount     int      `json:"frame_count"`
	Kinds          []string `json:"kinds"`
	TrackIDs       []int    `json:"track_ids"`
	PeakFrame      int      `json:"peak_frame"`
	PeakRatio      float64  `json:"peak_ratio"`
	Conclusion     string   `json:"conclusion"`
	Classification string   `json:"classification"`
}

// SourceSummary aggregates pixels assigned to an inferred source class.
type SourceSummary struct {
	Source       SourceClass `json:"source"`
	Verdict      Verdict     `json:"verdict"`
	Frames       int         `json:"frames"`
	RegionCount  int         `json:"region_count"`
	PixelCount   int64       `json:"pixel_count"`
	FirstFrame   int         `json:"first_frame"`
	LastFrame    int         `json:"last_frame"`
	ExampleFrame int         `json:"example_frame"`
	ExampleRect  Rect        `json:"example_rect"`
}

// StyleInterval is a run with strong legal-range shadow/highlight clipping.
type StyleInterval struct {
	FirstFrame      int     `json:"first_frame"`
	LastFrame       int     `json:"last_frame"`
	StartSeconds    float64 `json:"start_seconds"`
	EndSeconds      float64 `json:"end_seconds"`
	FrameCount      int     `json:"frame_count"`
	BlackWallRatio  float64 `json:"black_wall_ratio"`
	WhiteWallRatio  float64 `json:"white_wall_ratio"`
	OutOfRangeRatio float64 `json:"out_of_range_ratio"`
	Interpretation  string  `json:"interpretation"`
}

// FPSReport contains declared and observed timing evidence.
type FPSReport struct {
	DeclaredRational string   `json:"declared_rational,omitempty"`
	DeclaredFPS      float64  `json:"declared_fps,omitempty"`
	AverageFPS       float64  `json:"average_fps,omitempty"`
	ObservedMedian   float64  `json:"observed_median_fps,omitempty"`
	MinIntervalMS    float64  `json:"min_interval_ms,omitempty"`
	MaxIntervalMS    float64  `json:"max_interval_ms,omitempty"`
	VFRScore         float64  `json:"vfr_score"`
	FrameCount       int      `json:"frame_count"`
	PTSSource        string   `json:"pts_source,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

// Metadata is container/stream metadata relevant to range interpretation.
type Metadata struct {
	Path        string   `json:"path,omitempty"`
	Format      string   `json:"format,omitempty"`
	Codec       string   `json:"codec,omitempty"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	PixelFormat string   `json:"pixel_format,omitempty"`
	ColorRange  string   `json:"color_range,omitempty"`
	ColorSpace  string   `json:"color_space,omitempty"`
	Duration    float64  `json:"duration_seconds,omitempty"`
	NbFrames    int      `json:"nb_frames,omitempty"`
	FrameRate   Rational `json:"frame_rate,omitempty"`
	AverageRate Rational `json:"average_rate,omitempty"`
	Decoder     string   `json:"decoder,omitempty"`
}

// Report is the complete machine-readable analysis result.
type Report struct {
	SchemaVersion  string          `json:"schema_version"`
	Input          string          `json:"input"`
	Settings       Settings        `json:"settings"`
	LegalRanges    LegalRanges     `json:"legal_ranges"`
	Metadata       Metadata        `json:"metadata"`
	FPS            FPSReport       `json:"fps"`
	Frames         []FrameReport   `json:"frames"`
	Tracks         []Track         `json:"tracks,omitempty"`
	Segments       []Segment       `json:"segments,omitempty"`
	Sources        []SourceSummary `json:"sources,omitempty"`
	StyleIntervals []StyleInterval `json:"style_intervals,omitempty"`
	Conclusions    []string        `json:"conclusions"`
	ManualChecks   []string        `json:"manual_checks,omitempty"`
}

// LegalRanges records the exact code-value bounds used for statistics.
type LegalRanges struct {
	Bits   int         `json:"bits"`
	Luma   LevelBounds `json:"luma"`
	Chroma LevelBounds `json:"chroma"`
}
