package hdrframe

// Input is one frame-accurate HDR10+ metadata reconciliation timeline.
type Input struct {
	SchemaVersion    int           `json:"schema_version,omitempty"`
	Name             string        `json:"name,omitempty"`
	FPS              string        `json:"fps,omitempty"`
	Timebase         string        `json:"timebase,omitempty"`
	Policy           Policy        `json:"policy,omitempty"`
	Targets          []Target      `json:"targets,omitempty"`
	Frames           []Target      `json:"frames,omitempty"`
	Metadata         []Metadata    `json:"metadata,omitempty"`
	Records          []Metadata    `json:"records,omitempty"`
	ExpectedMetadata []Metadata    `json:"expected_metadata,omitempty"`
	SourceMetadata   []Metadata    `json:"source_metadata,omitempty"`
	ExpectedRecords  []Metadata    `json:"expected_records,omitempty"`
	PlayerEvents     []PlayerEvent `json:"player_events,omitempty"`
	PlayerLog        []PlayerEvent `json:"player_log,omitempty"`
}

// Target identifies a presentation frame that must be audited.
type Target struct {
	Frame           int64   `json:"frame"`
	Index           int64   `json:"index,omitempty"`
	PTS             int64   `json:"pts,omitempty"`
	PTSSeconds      float64 `json:"pts_seconds,omitempty"`
	TimeSeconds     float64 `json:"time_seconds,omitempty"`
	Timecode        string  `json:"timecode,omitempty"`
	Timescale       int64   `json:"timescale,omitempty"`
	ExpectedSceneID *int64  `json:"expected_scene_id,omitempty"`
	ExpectedScene   *int64  `json:"expected_scene,omitempty"`
	Label           string  `json:"label,omitempty"`
}

// Policy controls ambiguous stream semantics. The defaults favour the latest
// effective record and the common decoder behaviour of holding the last static
// metadata when a per-frame record is absent.
type Policy struct {
	Duplicate         string  `json:"duplicate,omitempty"`
	MissingMetadata   string  `json:"missing_metadata,omitempty"`
	SequenceGap       string  `json:"sequence_gap,omitempty"`
	TimeToleranceSecs float64 `json:"time_tolerance_seconds,omitempty"`
	StrictScene       bool    `json:"strict_scene,omitempty"`
}

// Metadata is one HDR10+ dynamic metadata version.
type Metadata struct {
	ID                 string          `json:"id,omitempty"`
	Version            string          `json:"version,omitempty"`
	ParameterVersion   string          `json:"parameter_version,omitempty"`
	Sequence           int64           `json:"sequence,omitempty"`
	SequenceNumber     int64           `json:"sequence_number,omitempty"`
	Frame              *int64          `json:"frame,omitempty"`
	FrameIndex         *int64          `json:"frame_index,omitempty"`
	SequenceFrameIndex *int64          `json:"sequence_frame_index,omitempty"`
	SceneID            *int64          `json:"scene_id,omitempty"`
	SceneNumber        *int64          `json:"scene_number,omitempty"`
	SceneFrameIndex    *int64          `json:"scene_frame_index,omitempty"`
	Effective          *TimeRef        `json:"effective,omitempty"`
	EffectiveAt        *TimeRef        `json:"effective_at,omitempty"`
	EffectiveSeconds   *float64        `json:"effective_seconds,omitempty"`
	EffectivePTS       *int64          `json:"effective_pts,omitempty"`
	PTSTimescale       int64           `json:"pts_timescale,omitempty"`
	Timecode           string          `json:"timecode,omitempty"`
	IngestOrder        int             `json:"ingest_order,omitempty"`
	BitstreamOrder     int             `json:"bitstream_order,omitempty"`
	Checksum           string          `json:"checksum,omitempty"`
	Source             string          `json:"source,omitempty"`
	Highlight          HighlightParams `json:"highlight,omitempty"`
	Color              ColorParams     `json:"color,omitempty"`
	Values             map[string]any  `json:"values,omitempty"`
}

// TimeRef is an explicit effective timestamp.
type TimeRef struct {
	Seconds   float64 `json:"seconds,omitempty"`
	PTS       int64   `json:"pts,omitempty"`
	Timescale int64   `json:"timescale,omitempty"`
	Frame     *int64  `json:"frame,omitempty"`
	Timecode  string  `json:"timecode,omitempty"`
}

// HighlightParams contains the controls most likely to affect local brightness.
type HighlightParams struct {
	TargetPeakNits float64 `json:"target_peak_nits,omitempty"`
	PeakNits       float64 `json:"peak_nits,omitempty"`
	MaxLuminance   float64 `json:"max_luminance,omitempty"`
	Gain           float64 `json:"gain,omitempty"`
	KneeX          float64 `json:"knee_x,omitempty"`
	KneeY          float64 `json:"knee_y,omitempty"`
}

// ColorParams contains controls most likely to affect hue and saturation.
type ColorParams struct {
	HueShiftDegrees float64 `json:"hue_shift_degrees,omitempty"`
	Saturation      float64 `json:"saturation,omitempty"`
	Warmth          float64 `json:"warmth,omitempty"`
}

// PlayerEvent is direct evidence of the metadata version used by a player.
type PlayerEvent struct {
	TargetFrame     int64   `json:"target_frame,omitempty"`
	Frame           int64   `json:"frame,omitempty"`
	AppliedVersion  string  `json:"applied_version,omitempty"`
	Version         string  `json:"version,omitempty"`
	AppliedSequence *int64  `json:"applied_sequence,omitempty"`
	Sequence        *int64  `json:"sequence,omitempty"`
	AppliedChecksum string  `json:"applied_checksum,omitempty"`
	Checksum        string  `json:"checksum,omitempty"`
	PTSSeconds      float64 `json:"pts_seconds,omitempty"`
	Source          string  `json:"source,omitempty"`
	Evidence        string  `json:"evidence,omitempty"`
	Note            string  `json:"note,omitempty"`
}

// Issue is a machine-readable reconciliation diagnostic.
type Issue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Frame    int64  `json:"frame,omitempty"`
	SceneID  *int64 `json:"scene_id,omitempty"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}
