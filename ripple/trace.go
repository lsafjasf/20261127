package ripple

// Change is one old -> new recalculation for a clip, effect, automation
// point, marker, or duration ledger.
type Change struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Detail    string `json:"detail,omitempty"`
	OldStart  Tick   `json:"oldStart,omitempty"`
	NewStart  Tick   `json:"newStart,omitempty"`
	OldEnd    Tick   `json:"oldEnd,omitempty"`
	NewEnd    Tick   `json:"newEnd,omitempty"`
	OldTime   Tick   `json:"oldTime,omitempty"`
	NewTime   Tick   `json:"newTime,omitempty"`
	OldClipID string `json:"oldClipId,omitempty"`
	NewClipID string `json:"newClipId,omitempty"`
	Reason    string `json:"reason"`
}

type Record struct {
	Index          int          `json:"index"`
	Mode           Mode         `json:"mode"`
	At             Tick         `json:"at"`
	EditDuration   Tick         `json:"editDuration"`
	TargetTrackID  string       `json:"targetTrackId,omitempty"`
	NewClipID      string       `json:"newClipId,omitempty"`
	MarkerPolicy   MarkerPolicy `json:"markerPolicy,omitempty"`
	RippleDelta    Tick         `json:"rippleDelta"`
	DurationDelta  Tick         `json:"durationDelta"`
	BeforeDuration Tick         `json:"beforeDuration"`
	AfterDuration  Tick         `json:"afterDuration"`
	DurationLedger Tick         `json:"durationLedger"`
	RippleTrackIDs []string     `json:"rippleTrackIds,omitempty"`
	Changes        []Change     `json:"changes"`
}

type MarkerPoint struct {
	Op      int    `json:"op"`
	OldTime Tick   `json:"oldTime"`
	NewTime Tick   `json:"newTime"`
	Reason  string `json:"reason"`
}

type MarkerTrace struct {
	ID     string        `json:"id"`
	Label  string        `json:"label,omitempty"`
	Points []MarkerPoint `json:"points"`
}

type Audit struct {
	Seed            int64                   `json:"seed"`
	Iterations      int                     `json:"iterations"`
	Mode            Mode                    `json:"mode"`
	InitialDuration Tick                    `json:"initialDuration"`
	FinalDuration   Tick                    `json:"finalDuration"`
	DurationLedger  Tick                    `json:"durationLedger"`
	Records         []Record                `json:"records"`
	MarkerTraces    map[string]*MarkerTrace `json:"markerTraces"`
	Warnings        []string                `json:"warnings,omitempty"`
}

func newAudit(mode Mode, seed int64, iterations int, initial Tick) *Audit {
	return &Audit{
		Seed:            seed,
		Iterations:      iterations,
		Mode:            mode,
		InitialDuration: initial,
		FinalDuration:   initial,
		DurationLedger:  initial,
		MarkerTraces:    make(map[string]*MarkerTrace),
	}
}

func (a *Audit) InitMarkerTraces(tl Timeline) {
	for _, marker := range tl.Markers {
		a.recordMarker(-1, marker, marker.Time, marker.Time, "initial")
	}
}

func (a *Audit) marker(id, label string) *MarkerTrace {
	trace := a.MarkerTraces[id]
	if trace == nil {
		trace = &MarkerTrace{ID: id, Label: label}
		a.MarkerTraces[id] = trace
	}
	return trace
}

func (a *Audit) recordMarker(op int, marker Marker, oldTime, newTime Tick, reason string) {
	trace := a.marker(marker.ID, marker.Label)
	trace.Points = append(trace.Points, MarkerPoint{
		Op:      op,
		OldTime: oldTime,
		NewTime: newTime,
		Reason:  reason,
	})
}
