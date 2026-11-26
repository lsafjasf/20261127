package laddercheck

// SchemaVersion tags all emitted reports and constraints.
const SchemaVersion = "laddercheck/v1"

// KeyframeFact is one sync/keyframe sample on all relevant timelines.
type KeyframeFact struct {
	// Sample is the zero-based sample number in the video track.
	Sample int `json:"sample"`
	// DTS is the decode timestamp in seconds on the media timeline.
	DTS Rat `json:"dts"`
	// PTS is DTS plus composition offset, in seconds on the media timeline.
	PTS Rat `json:"pts"`
	// DisplayPTS is the movie/display timestamp after applying the edit list.
	DisplayPTS Rat `json:"display_pts"`
	// SyncSample reports membership in stss (an explicit sync sample).
	SyncSample bool `json:"sync_sample"`
	// IDR is true for a closed IDR picture, false for a known non-IDR
	// keyframe (CRA/BLA/open-GOP/non-IDR I), nil when the bitstream was not
	// inspected or could not be classified.
	IDR *bool `json:"idr,omitempty"`
	// NALTypes lists the parameter/picture NAL unit types found in the sample.
	NALTypes []int `json:"nal_types,omitempty"`
	// Size is the sample size in bytes.
	Size int `json:"size_bytes,omitempty"`
}

// EditFact is one ISO-BMFF edit list entry (elst).
type EditFact struct {
	// Empty marks an empty edit (media_time == -1), e.g. initial priming delay.
	Empty bool `json:"empty"`
	// SegmentDuration is the edit length in movie-timeline seconds.
	SegmentDuration Rat `json:"segment_duration"`
	// MediaTime is the start offset on the media timeline; nil for empty edits.
	MediaTime *Rat `json:"media_time,omitempty"`
	// MediaRate is the playback rate (typically 1).
	MediaRate Rat `json:"media_rate,omitempty"`
	// RawMediaTimeTicks preserves the signed encoded media_time (-1 = empty).
	RawMediaTimeTicks int64 `json:"raw_media_time_ticks,omitempty"`
}

// RenditionFacts is the normalized comparison record for one ladder rung.
// It is produced either by parsing a real ISO-BMFF file or from JSON evidence.
type RenditionFacts struct {
	ID   string `json:"id"`
	Role string `json:"role,omitempty"`
	Path string `json:"path,omitempty"`

	Container   string `json:"container,omitempty"`
	Brand       string `json:"brand,omitempty"`
	Codec       string `json:"codec,omitempty"`
	CodecFamily string `json:"codec_family,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`

	MovieTimescale uint32 `json:"movie_timescale,omitempty"`
	MediaTimescale uint32 `json:"media_timescale,omitempty"`

	SampleCount           int  `json:"sample_count"`
	FPS                   Rat  `json:"fps,omitempty"`
	FrameDuration         Rat  `json:"frame_duration,omitempty"`
	VariableFrameDuration bool `json:"variable_frame_duration,omitempty"`

	// FirstDTS is sample 0 decode time, media timeline seconds.
	FirstDTS Rat `json:"first_dts"`
	// FirstPTS is sample 0 composition time, media timeline seconds.
	FirstPTS Rat `json:"first_pts"`
	// EarliestPTS is the minimum composition time (can precede sample 0 in
	// decode order when composition offsets are positive).
	EarliestPTS Rat `json:"earliest_pts"`
	// FirstDisplayPTS is the first presented frame on the movie timeline.
	FirstDisplayPTS Rat `json:"first_display_pts"`

	EditListPresent bool       `json:"edit_list_present"`
	Edits           []EditFact `json:"edits,omitempty"`

	Keyframes []KeyframeFact `json:"keyframes"`

	// GOPDisplayDurations are consecutive keyframe gaps on the display timeline.
	GOPDisplayDurations []Rat `json:"gop_display_durations,omitempty"`
	// GOPMediaDurations are consecutive keyframe gaps on the media timeline.
	GOPMediaDurations []Rat `json:"gop_media_durations,omitempty"`
	GOPUniform        bool  `json:"gop_uniform"`

	Warnings []string `json:"warnings,omitempty"`
}

// ConstraintOptions tunes unified constraint generation and conformance.
type ConstraintOptions struct {
	// GOPSeconds forces the unified keyframe interval. Zero means infer.
	GOPSeconds Rat `json:"gop_seconds,omitempty"`
	// FPS pins the common frame rate used to express GOPFrames.
	FPS Rat `json:"fps,omitempty"`
	// GOPFrames pins the GOP length in frames; combined with FPS.
	GOPFrames int64 `json:"gop_frames,omitempty"`
	// ToleranceSeconds is the allowed keyframe-time mismatch. Zero -> 1ms.
	ToleranceSeconds Rat `json:"tolerance_seconds,omitempty"`
	// OriginSeconds is the unified grid origin. Zero -> 0.
	OriginSeconds Rat `json:"origin_seconds,omitempty"`
	// RequireIDR demands true closed IDR samples (default true).
	RequireIDR *bool `json:"require_idr,omitempty"`
	// HorizonSeconds optionally bounds emitted grid keyframe times.
	HorizonSeconds Rat `json:"horizon_seconds,omitempty"`
}

// KeyframeConstraint is the machine-readable unified keyframe contract.
type KeyframeConstraint struct {
	Schema            string   `json:"schema"`
	OriginSeconds     Rat      `json:"origin_seconds"`
	GOPSeconds        Rat      `json:"gop_seconds"`
	GOPFrames         int64    `json:"gop_frames,omitempty"`
	FPS               Rat      `json:"fps,omitempty"`
	ToleranceSeconds  Rat      `json:"tolerance_seconds"`
	RequireIDR        bool     `json:"require_idr"`
	ClosedGOP         bool     `json:"closed_gop"`
	AlignTimeline     string   `json:"align_timeline"`
	EditListPolicy    string   `json:"edit_list_policy"`
	FirstFramePolicy  string   `json:"first_frame_policy"`
	GridKeyframeTimes []Rat    `json:"grid_keyframe_times,omitempty"`
	EncoderHints      []string `json:"encoder_hints,omitempty"`
	Source            string   `json:"source"`
}

// Status values for a rendition verdict.
const (
	StatusCompliant    = "compliant"
	StatusRemuxOnly    = "remux_only_fixable"
	StatusReencode     = "requires_reencode"
	StatusInsufficient = "insufficient_evidence"
)

// Verdict is the per-rung conformance and remediation classification.
type Verdict struct {
	ID   string `json:"id"`
	Role string `json:"role,omitempty"`

	Status          string `json:"status"`
	RemuxPossible   bool   `json:"remux_possible_without_reencode"`
	DisplayAligned  bool   `json:"display_aligned"`
	ContentAligned  bool   `json:"content_pts_grid_aligned"`
	SamplesAligned  bool   `json:"sample_indices_aligned"`
	AllKeyframesIDR bool   `json:"all_keyframes_idr"`
	GOPUniform      bool   `json:"gop_uniform"`
	FPSCompatible   bool   `json:"fps_compatible"`

	FirstDisplayPTS Rat `json:"first_display_pts"`
	// LabelOffsetSeconds is the constant container/elst/first-PTS shift that
	// currently separates this rung's media keyframes from the unified grid.
	LabelOffsetSeconds *Rat `json:"label_offset_seconds,omitempty"`
	// MaxDisplayError is the worst keyframe distance to a grid point.
	MaxDisplayError Rat `json:"max_display_error_seconds"`
	// MaxMediaCadenceError measures GOP interval deviation from GOPSeconds.
	MaxMediaCadenceError Rat `json:"max_media_cadence_error_seconds"`

	GridKeyframeCount int   `json:"grid_keyframe_count_in_range"`
	MatchedCount      int   `json:"matched_keyframe_count"`
	MissingGridTimes  []Rat `json:"missing_grid_times,omitempty"`
	OffGridTimes      []Rat `json:"off_grid_times,omitempty"`
	NonIDRSamples     []int `json:"non_idr_samples,omitempty"`

	Reasons []string `json:"reasons"`
	Actions []string `json:"actions"`
}

// Summary aggregates the headline answer.
type Summary struct {
	Renditions               int      `json:"renditions"`
	Compliant                []string `json:"compliant"`
	RemuxOnly                []string `json:"remux_only_fixable"`
	RequiresReencode         []string `json:"requires_reencode"`
	Insufficient             []string `json:"insufficient_evidence"`
	SwitchSafe               bool     `json:"switch_safe_now"`
	AlignableWithoutReencode bool     `json:"alignable_without_reencode"`
}

// Report is the complete comparison + constraint + verdicts document.
type Report struct {
	Schema     string             `json:"schema"`
	Input      string             `json:"input,omitempty"`
	Constraint KeyframeConstraint `json:"constraint"`
	Facts      []RenditionFacts   `json:"facts"`
	Verdicts   []Verdict          `json:"verdicts"`
	Summary    Summary            `json:"summary"`
}
