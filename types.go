package tailtrace

// Frame is one source, renderer-output, or player-observed frame.
// PTS is expressed in seconds but remains an exact rational value.
type Frame struct {
	Index       int64     `json:"index"`
	PTS         *Rational `json:"pts,omitempty"`
	Black       bool      `json:"black,omitempty"`
	Origin      string    `json:"origin,omitempty"`
	SourceIndex *int64    `json:"source_index,omitempty"`
	PTSTicks    *int64    `json:"pts_ticks,omitempty"`
	TimeBase    *Rational `json:"time_base,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// PlayerFrame is optional wall-clock evidence from a player. Times are
// relative to the start of the playback timeline.
type PlayerFrame struct {
	Index      int64     `json:"index"`
	Presented  bool      `json:"presented"`
	ShownStart *Rational `json:"shown_start,omitempty"`
	ShownEnd   *Rational `json:"shown_end,omitempty"`
}

// PlayerEvidence contains player UI/display evidence.
type PlayerEvidence struct {
	ReportedDuration *Rational     `json:"reported_duration,omitempty"`
	EditListEnd      *Rational     `json:"edit_list_end,omitempty"`
	Frames           []PlayerFrame `json:"frames,omitempty"`
}

// Contract is the delivery/output contract and optional muxer metadata.
type Contract struct {
	FPS                    Rational       `json:"fps"`
	StartPTS               Rational       `json:"start_pts,omitempty"`
	ContentFrames          int64          `json:"content_frames"`
	DeliveryFrames         *int64         `json:"delivery_frames,omitempty"`
	DeliveryDuration       *Rational      `json:"delivery_duration,omitempty"`
	AllowedTailBlackFrames int64          `json:"allowed_tail_black_frames"`
	TailPolicy             string         `json:"tail_policy,omitempty"`
	ContainerDuration      *Rational      `json:"container_duration,omitempty"`
	AudioDuration          *Rational      `json:"audio_duration,omitempty"`
	Player                 PlayerEvidence `json:"player,omitempty"`
	Tolerance              *Rational      `json:"time_tolerance,omitempty"`
}

// Evidence is the input to Analyze. A JSON file may be assembled from frame
// PTS logs, black-frame detection, renderer manifests, and player traces.
type Evidence struct {
	Contract    Contract  `json:"contract"`
	Source      []Frame   `json:"source,omitempty"`
	Output      []Frame   `json:"output"`
	PTSTimeBase *Rational `json:"pts_time_base,omitempty"`
}

// FrameTrace is the reconciled PTS and presentation interval for one output.
type FrameTrace struct {
	Index           int64     `json:"index"`
	Role            string    `json:"role"`
	Origin          string    `json:"origin,omitempty"`
	Black           bool      `json:"black"`
	PTS             Rational  `json:"pts"`
	ExpectedPTS     Rational  `json:"expected_pts"`
	PTSDrift        Rational  `json:"pts_drift"`
	DisplayStart    Rational  `json:"display_start"`
	DisplayEnd      Rational  `json:"display_end"`
	PTSTicks        *int64    `json:"pts_ticks,omitempty"`
	TimeBase        *Rational `json:"time_base,omitempty"`
	SourceIndex     *int64    `json:"source_index,omitempty"`
	PlayerPresented *bool     `json:"player_presented,omitempty"`
}

// Durations collects the several "duration" numbers that are often confused.
type Durations struct {
	FPS               Rational  `json:"fps"`
	FrameDuration     Rational  `json:"frame_duration"`
	ContentFrames     int64     `json:"content_frames"`
	ContentDuration   Rational  `json:"content_duration"`
	OutputFrames      int64     `json:"output_frames"`
	LastPTS           Rational  `json:"last_pts"`
	LastPTSEnd        Rational  `json:"last_pts_end"`
	CFRDuration       Rational  `json:"cfr_duration"`
	TailBlackFrames   int64     `json:"tail_black_frames"`
	TailBlackStartPTS *Rational `json:"tail_black_start_pts,omitempty"`
	TailBlackDuration Rational  `json:"tail_black_duration"`
	ContractFrames    int64     `json:"contract_frames"`
	ContractDuration  Rational  `json:"contract_duration"`
	PlayerScheduleEnd Rational  `json:"player_schedule_end"`
	PlayerReportedEnd *Rational `json:"player_reported_end,omitempty"`
	EditListEnd       *Rational `json:"edit_list_end,omitempty"`
	ContainerDuration *Rational `json:"container_duration,omitempty"`
	AudioDuration     *Rational `json:"audio_duration,omitempty"`
}

// Finding is one contract/PTS/player reconciliation result.
type Finding struct {
	Code             string    `json:"code"`
	Severity         string    `json:"severity"`
	Message          string    `json:"message"`
	ExtraFrames      int64     `json:"extra_frames,omitempty"`
	FirstExtraIndex  int64     `json:"first_extra_index,omitempty"`
	FirstExtraPTS    *Rational `json:"first_extra_pts,omitempty"`
	ExtraDuration    *Rational `json:"extra_duration,omitempty"`
	ExpectedDuration *Rational `json:"expected_duration,omitempty"`
	ActualDuration   *Rational `json:"actual_duration,omitempty"`
}

// Report is the complete diagnosis.
type Report struct {
	OK             bool         `json:"ok"`
	Classification string       `json:"classification"`
	Summary        string       `json:"summary"`
	Frames         []FrameTrace `json:"frames"`
	Durations      Durations    `json:"durations"`
	Findings       []Finding    `json:"findings"`
}

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"

	RoleContent       = "content"
	RoleTailBlackPad  = "tail_black_padding"
	RoleExtraUnknown  = "extra_unknown"
	RoleMissingOutput = "missing_output"
)
