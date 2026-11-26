package align

// Kind describes the evidence used by an event.
type Kind string

const (
	KindClap    Kind = "clap"
	KindFlash   Kind = "flash"
	KindAmbient Kind = "ambient"
	KindMarker  Kind = "marker"
)

// Event is a physical stimulus that can be observed by more than one device.
// MasterTime is optional: when absent it can be inferred from the master
// device or propagated through an already-locked camera.
type Event struct {
	ID         string   `json:"id"`
	Kind       Kind     `json:"kind"`
	MasterTime *float64 `json:"master_time,omitempty"`
	Note       string   `json:"note,omitempty"`
}

// Observation is one device's detection of an event in its own local clock.
type Observation struct {
	Event      string   `json:"event"`
	Local      float64  `json:"local"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// Segment is a continuous recording interval. A camera restart starts a new
// segment and therefore a new, independently fitted clock domain.
type Segment struct {
	ID           string        `json:"id"`
	Start        float64       `json:"start"`
	End          float64       `json:"end"`
	Observations []Observation `json:"observations,omitempty"`
}

// Device is a camera or a recorder. The device identified by
// Session.MasterDevice defines the master timeline.
type Device struct {
	ID       string    `json:"id"`
	Role     string    `json:"role,omitempty"`
	FPS      float64   `json:"fps,omitempty"`
	Segments []Segment `json:"segments"`
}

// Session is the complete evidence manifest. All times are seconds.
type Session struct {
	MasterDevice string   `json:"master_device"`
	Events       []Event  `json:"events"`
	Devices      []Device `json:"devices"`
	Config       Config   `json:"config,omitempty"`
}

// Config controls evidence acceptance, robust fitting, and coverage limits.
// Zero values are replaced by defaults during Normalize.
type Config struct {
	MinAnchorConfidence     float64 `json:"min_anchor_confidence"`
	MinOffsetAnchors        int     `json:"min_offset_anchors"`
	MinDriftAnchors         int     `json:"min_drift_anchors"`
	MinDriftBaselineSec     float64 `json:"min_drift_baseline_sec"`
	MaxDriftPPM             float64 `json:"max_drift_ppm"`
	AnchorToleranceMS       float64 `json:"anchor_tolerance_ms"`
	BaseUncertaintyMS       float64 `json:"base_uncertainty_ms"`
	MaxUnverifiedDriftPPM   float64 `json:"max_unverified_drift_ppm"`
	ConfirmedDriftPPM       float64 `json:"confirmed_drift_uncertainty_ppm"`
	MaxAnchorGapSec         float64 `json:"max_anchor_gap_sec"`
	MaxExtrapolationSec     float64 `json:"max_extrapolation_sec"`
	MaxMappingErrorMS       float64 `json:"max_mapping_error_ms"`
	PropagationFactor       float64 `json:"propagation_factor"`
	AmbientConfidenceFactor float64 `json:"ambient_confidence_factor"`
}
