package hdrswitch

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"

	SegmentBefore = "before_switch"
	SegmentAfter  = "after_switch"

	TransferHLG = "HLG"
	TransferPQ  = "PQ"

	PrimariesBT709  = "BT.709"
	PrimariesBT2020 = "BT.2020"
	PrimariesP3D65  = "P3-D65"
	PrimariesDCIP3  = "DCI-P3"
)

// Evidence is the AU-level color/GOP timeline. It can be assembled from an
// HEVC/AV1 parser, SEI/VUI export, FFprobe side data, or decoder logs.
type Evidence struct {
	SwitchAt               Rational       `json:"switch_at"`
	ExpectedBeforeTransfer string         `json:"expected_before_transfer,omitempty"`
	ExpectedAfterTransfer  string         `json:"expected_after_transfer,omitempty"`
	FrameRate              *Rational      `json:"frame_rate,omitempty"`
	PTSTimeBase            *Rational      `json:"pts_time_base,omitempty"`
	BitDepth               int            `json:"bit_depth,omitempty"`
	FullRange              *bool          `json:"full_range,omitempty"`
	Policy                 Policy         `json:"policy,omitempty"`
	Player                 PlayerEvidence `json:"player,omitempty"`
	AccessUnits            []AccessUnit   `json:"access_units"`
	TimelineComplete       bool           `json:"timeline_complete,omitempty"`
}

type Policy struct {
	RequireIDR                *bool `json:"require_idr,omitempty"`
	RequireParameterSetUpdate *bool `json:"require_parameter_set_update,omitempty"`
	CheckStickyPlayer         *bool `json:"check_sticky_player,omitempty"`
}

type PlayerEvidence struct {
	UsesFirstFrameMetadata bool           `json:"uses_first_frame_metadata,omitempty"`
	StickyMetadata         *ColorMetadata `json:"sticky_metadata,omitempty"`
}

type AccessUnit struct {
	Index          int64          `json:"index"`
	PTS            *Rational      `json:"pts,omitempty"`
	PTSTicks       *int64         `json:"pts_ticks,omitempty"`
	TimeBase       *Rational      `json:"time_base,omitempty"`
	DTS            *Rational      `json:"dts,omitempty"`
	GOPID          string         `json:"gop_id,omitempty"`
	IDR            bool           `json:"idr,omitempty"`
	RandomAccess   bool           `json:"random_access,omitempty"`
	RAPType        string         `json:"rap_type,omitempty"`
	ClosedGOP      *bool          `json:"closed_gop,omitempty"`
	CrossReference bool           `json:"cross_boundary_reference,omitempty"`
	ParameterSets  bool           `json:"parameter_sets_updated,omitempty"`
	Metadata       *ColorMetadata `json:"metadata,omitempty"`
	Note           string         `json:"note,omitempty"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type MasteringVolume struct {
	Primaries        string    `json:"primaries,omitempty"`
	PrimariesXY      []Point   `json:"primaries_xy,omitempty"`
	WhitePoint       *Point    `json:"white_point,omitempty"`
	MaxLuminanceNits *Rational `json:"max_luminance_nits,omitempty"`
	MinLuminanceNits *Rational `json:"min_luminance_nits,omitempty"`
}

// ColorMetadata is the effective color description for one AU. Pointer and
// zero fields are treated as "unchanged" when merging an AU patch.
type ColorMetadata struct {
	Transfer        string           `json:"transfer,omitempty"`
	TransferCode    int              `json:"transfer_code,omitempty"`
	Primaries       string           `json:"primaries,omitempty"`
	PrimariesCode   int              `json:"primaries_code,omitempty"`
	Matrix          string           `json:"matrix,omitempty"`
	MatrixCode      int              `json:"matrix_code,omitempty"`
	FullRange       *bool            `json:"full_range,omitempty"`
	BitDepth        int              `json:"bit_depth,omitempty"`
	NominalPeakNits *Rational        `json:"nominal_peak_nits,omitempty"`
	MaxCLL          *Rational        `json:"max_cll,omitempty"`
	MaxFALL         *Rational        `json:"max_fall,omitempty"`
	Mastering       *MasteringVolume `json:"mastering,omitempty"`
}

type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	AUIndex  *int64 `json:"au_index,omitempty"`
}

type IntRange struct {
	Min   int    `json:"min"`
	Max   int    `json:"max"`
	Unit  string `json:"unit"`
	Basis string `json:"basis,omitempty"`
}

type FloatRange struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Unit  string  `json:"unit"`
	Basis string  `json:"basis,omitempty"`
}

type CodeRange struct {
	BitDepth     int      `json:"bit_depth"`
	Quantization string   `json:"quantization"`
	Luma         IntRange `json:"luma"`
	Chroma       IntRange `json:"chroma"`
	RGB          IntRange `json:"rgb"`
}

type PeakSummary struct {
	NominalPeakNits      *Rational `json:"nominal_peak_nits,omitempty"`
	MaxCLL               *Rational `json:"max_cll,omitempty"`
	MaxFALL              *Rational `json:"max_fall,omitempty"`
	MasteringMaxNits     *Rational `json:"mastering_max_nits,omitempty"`
	MasteringMinNits     *Rational `json:"mastering_min_nits,omitempty"`
	PQRepresentationMax  float64   `json:"pq_representation_max_nits,omitempty"`
	HLGSystemGamma       *float64  `json:"hlg_system_gamma,omitempty"`
	EffectiveCeilingNits Rational  `json:"effective_ceiling_nits"`
	CeilingBasis         string    `json:"ceiling_basis"`
}

type GamutRange struct {
	Primaries      string     `json:"primaries"`
	PrimariesCode  int        `json:"primaries_code,omitempty"`
	Red            Point      `json:"red"`
	Green          Point      `json:"green"`
	Blue           Point      `json:"blue"`
	White          Point      `json:"white"`
	CIECoordinates FloatRange `json:"cie_coordinates"`
	SEICoordinates IntRange   `json:"sei_coordinates"`
}

type LegalEnvelope struct {
	Transfer                string      `json:"transfer"`
	H273TransferCode        int         `json:"h273_transfer_code"`
	ComponentCodes          CodeRange   `json:"component_codes"`
	NonlinearSignal         FloatRange  `json:"nonlinear_signal"`
	SceneLinearSignal       *FloatRange `json:"scene_linear_signal,omitempty"`
	AbsoluteLuminanceFixed  bool        `json:"absolute_luminance_fixed"`
	RepresentationLuminance *FloatRange `json:"representation_luminance,omitempty"`
	ReferenceLuminance      *FloatRange `json:"reference_luminance,omitempty"`
	Peak                    PeakSummary `json:"peak"`
	Gamut                   GamutRange  `json:"gamut"`
}

type Boundary struct {
	AUIndex int64    `json:"au_index"`
	PTS     Rational `json:"pts"`
	Type    string   `json:"type"`
	IDR     bool     `json:"idr"`
	Safe    bool     `json:"safe_for_metadata_switch"`
}

type GOPRisk struct {
	AtRisk                 bool      `json:"at_risk"`
	Safe                   bool      `json:"safe"`
	CutAUIndex             int64     `json:"cut_au_index"`
	CutPTS                 Rational  `json:"cut_pts"`
	GOPID                  string    `json:"gop_id,omitempty"`
	GOPStartAUIndex        *int64    `json:"gop_start_au_index,omitempty"`
	GOPStartPTS            *Rational `json:"gop_start_pts,omitempty"`
	PositionFromGOPStart   int64     `json:"position_from_gop_start,omitempty"`
	OpenGOP                bool      `json:"open_gop"`
	CrossBoundaryReference bool      `json:"cross_boundary_reference"`
	PreviousRandomAccess   *Boundary `json:"previous_random_access,omitempty"`
	NextRandomAccess       *Boundary `json:"next_random_access,omitempty"`
	Reason                 string    `json:"reason,omitempty"`
	RequiredAction         string    `json:"required_action,omitempty"`
}

type AUState struct {
	Index           int64         `json:"index"`
	Position        int           `json:"position"`
	PTS             Rational      `json:"pts"`
	DTS             *Rational     `json:"dts,omitempty"`
	Segment         string        `json:"segment"`
	GOPID           string        `json:"gop_id,omitempty"`
	IDR             bool          `json:"idr"`
	RandomAccess    bool          `json:"random_access"`
	BoundarySafe    bool          `json:"boundary_safe"`
	IsCutAU         bool          `json:"is_cut_au"`
	MetadataChanged bool          `json:"metadata_changed"`
	ChangedFields   []string      `json:"changed_fields,omitempty"`
	Metadata        ColorMetadata `json:"metadata"`
	PlayerStale     bool          `json:"player_stale"`
	StaleFields     []string      `json:"stale_fields,omitempty"`
}

type Regime struct {
	ID           int           `json:"id"`
	Segment      string        `json:"segment"`
	FirstAUIndex int64         `json:"first_au_index"`
	LastAUIndex  int64         `json:"last_au_index"`
	StartPTS     Rational      `json:"start_pts"`
	EndPTS       *Rational     `json:"end_pts,omitempty"`
	AUCount      int           `json:"au_count"`
	Metadata     ColorMetadata `json:"metadata"`
	Envelope     LegalEnvelope `json:"envelope"`
}

type SegmentReport struct {
	ID               string        `json:"id"`
	StartPTS         Rational      `json:"start_pts"`
	EndPTS           *Rational     `json:"end_pts,omitempty"`
	AUCount          int           `json:"au_count"`
	FirstAUIndex     int64         `json:"first_au_index"`
	LastAUIndex      int64         `json:"last_au_index"`
	Stable           bool          `json:"stable"`
	Transfers        []string      `json:"transfers"`
	Primaries        []string      `json:"primaries"`
	PeaksNits        []string      `json:"peaks_nits"`
	BoundaryMetadata ColorMetadata `json:"boundary_metadata"`
	LegalRange       LegalEnvelope `json:"legal_range"`
	RegimeIDs        []int         `json:"regime_ids"`
}

type PlayerReport struct {
	StickyFirstFrame bool           `json:"sticky_first_frame"`
	StickyMetadata   *ColorMetadata `json:"sticky_metadata,omitempty"`
	MismatchAUCount  int            `json:"mismatch_au_count"`
	FirstMismatchAU  *int64         `json:"first_mismatch_au,omitempty"`
	StaleFields      []string       `json:"stale_fields,omitempty"`
}

type Report struct {
	OK             bool           `json:"ok"`
	Classification string         `json:"classification"`
	Summary        string         `json:"summary"`
	SwitchAt       Rational       `json:"switch_at"`
	CutAUIndex     *int64         `json:"cut_au_index,omitempty"`
	CutAlignment   string         `json:"cut_alignment"`
	GOPRisk        *GOPRisk       `json:"gop_risk,omitempty"`
	Before         *SegmentReport `json:"before,omitempty"`
	After          *SegmentReport `json:"after,omitempty"`
	Regimes        []Regime       `json:"regimes"`
	Player         *PlayerReport  `json:"player,omitempty"`
	AccessUnits    []AUState      `json:"access_units"`
	Findings       []Finding      `json:"findings"`
}
