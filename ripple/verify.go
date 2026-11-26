package ripple

import "fmt"

type Verification struct {
	OK                  bool     `json:"ok"`
	DurationActual      Tick     `json:"durationActual"`
	DurationFromRecords Tick     `json:"durationFromRecords"`
	DurationDeltaSum    Tick     `json:"durationDeltaSum"`
	MarkersChecked      int      `json:"markersChecked"`
	MarkerHistoryPoints int      `json:"markerHistoryPoints"`
	Errors              []string `json:"errors,omitempty"`
}

func VerifyTrace(tl Timeline, audit *Audit) Verification {
	result := Verification{OK: true, DurationActual: tl.Duration()}
	if audit == nil {
		result.OK = false
		result.Errors = append(result.Errors, "nil audit")
		return result
	}

	ledger := audit.InitialDuration
	for index, record := range audit.Records {
		if record.Index != index {
			result.Errors = append(result.Errors, fmt.Sprintf("record %d has index %d", index, record.Index))
		}
		if record.BeforeDuration != ledger {
			result.Errors = append(result.Errors, fmt.Sprintf("op %d before duration %d does not match ledger %d", index, record.BeforeDuration, ledger))
		}
		if record.AfterDuration != record.BeforeDuration+record.DurationDelta {
			result.Errors = append(result.Errors, fmt.Sprintf("op %d duration delta does not add up", index))
		}
		if record.DurationLedger != record.AfterDuration {
			result.Errors = append(result.Errors, fmt.Sprintf("op %d ledger field does not match after duration", index))
		}
		ledger += record.DurationDelta
		result.DurationDeltaSum += record.DurationDelta
	}

	result.DurationFromRecords = ledger
	audit.DurationLedger = ledger
	audit.FinalDuration = tl.Duration()
	if ledger != tl.Duration() {
		result.Errors = append(result.Errors, fmt.Sprintf("duration ledger %d != recomputed timeline duration %d", ledger, tl.Duration()))
	}
	if audit.FinalDuration != ledger {
		result.Errors = append(result.Errors, "audit final duration does not match ledger")
	}

	markersByID := make(map[string]Marker, len(tl.Markers))
	for _, marker := range tl.Markers {
		markersByID[marker.ID] = marker
	}

	for id, trace := range audit.MarkerTraces {
		result.MarkerHistoryPoints += len(trace.Points)
		marker, ok := markersByID[id]
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("marker trace %s has no final marker", id))
			continue
		}
		result.MarkersChecked++
		if len(trace.Points) == 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("marker %s has no history", id))
			continue
		}

		first := trace.Points[0]
		if first.Op != -1 || first.OldTime != first.NewTime {
			result.Errors = append(result.Errors, fmt.Sprintf("marker %s lacks initial trace", id))
		}
		for i := 1; i < len(trace.Points); i++ {
			previous, current := trace.Points[i-1], trace.Points[i]
			if current.OldTime != previous.NewTime {
				result.Errors = append(result.Errors, fmt.Sprintf("marker %s history is discontinuous at op %d", id, current.Op))
			}
		}
		last := trace.Points[len(trace.Points)-1]
		if last.NewTime != marker.Time {
			result.Errors = append(result.Errors, fmt.Sprintf("marker %s final trace %d != timeline %d", id, last.NewTime, marker.Time))
		}
	}

	for _, marker := range tl.Markers {
		if _, ok := audit.MarkerTraces[marker.ID]; !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("marker %s is missing trace", marker.ID))
		}
	}

	result.OK = len(result.Errors) == 0
	return result
}
