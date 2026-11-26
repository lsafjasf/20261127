package align

// Report is the stable JSON/API contract produced by Solve.
type Report struct {
	Version      int          `json:"version"`
	MasterDevice string       `json:"master_device"`
	Timeline     string       `json:"timeline"`
	Config       Config       `json:"config"`
	Events       []EventInfo  `json:"events"`
	Devices      []DeviceInfo `json:"devices"`
	Diagnostics  []Diagnostic `json:"diagnostics,omitempty"`
}

// EventInfo records where an event's master time came from.
type EventInfo struct {
	ID            string   `json:"id"`
	Kind          Kind     `json:"kind"`
	MasterTime    *float64 `json:"master_time,omitempty"`
	Source        string   `json:"source"`
	Confidence    float64  `json:"confidence"`
	ObservationOf []string `json:"observed_by,omitempty"`
}

type DeviceInfo struct {
	ID       string          `json:"id"`
	Role     string          `json:"role,omitempty"`
	Master   bool            `json:"master"`
	Segments []SegmentReport `json:"segments"`
}

type SegmentReport struct {
	Device         string      `json:"device"`
	ID             string      `json:"segment"`
	LocalStart     float64     `json:"local_start"`
	LocalEnd       float64     `json:"local_end"`
	Status         string      `json:"status"`
	FirstTrusted   *AnchorInfo `json:"first_trusted_anchor,omitempty"`
	OffsetAtStart  *float64    `json:"offset_at_local_start_sec,omitempty"`
	DriftPPM       *float64    `json:"drift_ppm,omitempty"`
	DriftConfirmed bool        `json:"drift_confirmed"`
	Fit            *FitInfo    `json:"fit,omitempty"`
	Map            []MapPiece  `json:"map"`
	Indeterminate  []Interval  `json:"indeterminate"`
}

// AnchorInfo is an accepted evidence point in both local and master time.
type AnchorInfo struct {
	Event      string  `json:"event"`
	Kind       Kind    `json:"kind"`
	Local      float64 `json:"local"`
	Master     float64 `json:"master"`
	OffsetSec  float64 `json:"offset_sec"`
	Confidence float64 `json:"confidence"`
	ResidualMS float64 `json:"residual_ms"`
}

type FitInfo struct {
	Slope           float64 `json:"slope"`
	Intercept       float64 `json:"intercept"`
	AnchorCount     int     `json:"anchor_count"`
	RejectedAnchors int     `json:"rejected_anchors"`
	BaselineSec     float64 `json:"baseline_sec"`
	RMSResidualMS   float64 `json:"rms_residual_ms"`
	MaxResidualMS   float64 `json:"max_residual_ms"`
}

// MapPiece maps a half-open local interval [LocalStart,LocalEnd) using
// master = Slope*local + Intercept.
type MapPiece struct {
	LocalStart  float64 `json:"local_start"`
	LocalEnd    float64 `json:"local_end"`
	MasterStart float64 `json:"master_start"`
	MasterEnd   float64 `json:"master_end"`
	Slope       float64 `json:"slope"`
	Intercept   float64 `json:"intercept"`
	MaxErrorMS  float64 `json:"max_error_ms"`
}

// Interval is a half-open local interval that cannot be mapped reliably.
type Interval struct {
	LocalStart float64 `json:"local_start"`
	LocalEnd   float64 `json:"local_end"`
	Reason     string  `json:"reason"`
	Detail     string  `json:"detail,omitempty"`
}

type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Device   string `json:"device,omitempty"`
	Segment  string `json:"segment,omitempty"`
	Event    string `json:"event,omitempty"`
	Message  string `json:"message"`
}
