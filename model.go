package multicam

// Project is the input document. A sequence owns the edit timeline, machines
// capture clips through lenses, and claps carry the audio-sync observations.
type Project struct {
	Sequence Sequence  `json:"sequence"`
	Machines []Machine `json:"machines"`
	Lenses   []Lens    `json:"lenses"`
	Clips    []Clip    `json:"clips"`
	Claps    []Clap    `json:"claps"`
}

// Sequence defines the master timeline. Its origin is the common clap, so the
// master interval is [0, MasterDurationSeconds).
type Sequence struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name,omitempty"`
	MasterDurationSeconds float64 `json:"master_duration_seconds"`
	ToleranceSeconds      float64 `json:"tolerance_seconds"`
}

// Machine is one camera body. Its local clock is the time base used by Clip
// start/end values and ClapObservation.LocalSeconds.
type Machine struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Lens is mounted on exactly one machine. A machine may use several lenses in
// the same sequence.
type Lens struct {
	ID            string  `json:"id"`
	MachineID     string  `json:"machine_id"`
	Name          string  `json:"name,omitempty"`
	FocalLengthMM float64 `json:"focal_length_mm,omitempty"`
}

// Clip is a continuous recording on a machine-local clock. Gaps between clips
// are real recording downtime and are never filled automatically.
type Clip struct {
	ID           string  `json:"id"`
	SequenceID   string  `json:"sequence_id"`
	MachineID    string  `json:"machine_id"`
	LensID       string  `json:"lens_id"`
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
}

// Clap is one physical hand-clap heard by any number of machines. MasterSeconds
// is its position on the sequence timeline; observations are on each camera's
// independent local clock.
type Clap struct {
	ID            string            `json:"id"`
	MasterSeconds float64           `json:"master_seconds"`
	Observations  []ClapObservation `json:"observations"`
}

// ClapObservation records when one machine heard a shared clap. ErrorSeconds is
// the measurement uncertainty, not a correction to apply.
type ClapObservation struct {
	MachineID    string  `json:"machine_id"`
	ClipID       string  `json:"clip_id,omitempty"`
	LocalSeconds float64 `json:"local_seconds"`
	ErrorSeconds float64 `json:"error_seconds,omitempty"`
}

// Interval is a half-open time range [StartSeconds, EndSeconds).
type Interval struct {
	StartSeconds float64 `json:"start_seconds"`
	EndSeconds   float64 `json:"end_seconds"`
}

// Report is the deterministic result of analyzing a Project.
type Report struct {
	Sequence         ReportSequence  `json:"sequence"`
	ToleranceSeconds float64         `json:"tolerance_seconds"`
	Channels         []ChannelReport `json:"channels"`
}

type ReportSequence struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name,omitempty"`
	MasterStartSeconds float64 `json:"master_start_seconds"`
	MasterEndSeconds   float64 `json:"master_end_seconds"`
}

// ChannelReport contains the four-layer mapping for one machine, plus the
// merged intervals on the master timeline that the machine can still cover.
type ChannelReport struct {
	Machine         Machine          `json:"machine"`
	Status          string           `json:"status"`
	RejectionReason string           `json:"rejection_reason,omitempty"`
	Alignment       *AlignmentReport `json:"alignment,omitempty"`
	Coverage        []Interval       `json:"coverage"`
	CoveredSeconds  float64          `json:"covered_seconds"`
	Lenses          []LensReport     `json:"lenses"`
}

// AlignmentReport exposes the evidence used for a channel decision. Offset is
// applied only when Applied is true.
type AlignmentReport struct {
	Applied                    bool    `json:"applied"`
	OffsetSeconds              float64 `json:"offset_seconds"`
	ObservationCount           int     `json:"observation_count"`
	MaxObservationErrorSeconds float64 `json:"max_observation_error_seconds"`
	MaxResidualSeconds         float64 `json:"max_residual_seconds"`
	MaxErrorSeconds            float64 `json:"max_error_seconds"`
	ToleranceSeconds           float64 `json:"tolerance_seconds"`
}

type LensReport struct {
	Lens           Lens         `json:"lens"`
	Clips          []ClipReport `json:"clips"`
	Coverage       []Interval   `json:"coverage"`
	CoveredSeconds float64      `json:"covered_seconds"`
}

type ClipReport struct {
	ID               string    `json:"id"`
	LocalInterval    Interval  `json:"local_interval"`
	MasterInterval   *Interval `json:"master_interval,omitempty"`
	CoverageInterval *Interval `json:"coverage_interval,omitempty"`
}

const (
	StatusAligned           = "aligned"
	StatusToleranceExceeded = "tolerance_exceeded"
	StatusMissingClap       = "missing_clap"
	StatusNoClips           = "no_clips"
)
