package align

import (
	"math"
	"sort"
)

type segmentState struct {
	device  *Device
	segment *Segment
	anchors []anchor
	fit     *fitResult
	status  string
}

type diagnosticSet struct {
	list []Diagnostic
	seen map[string]bool
}

func (set *diagnosticSet) add(item Diagnostic) {
	key := item.Severity + "|" + item.Code + "|" + item.Device + "|" + item.Segment + "|" + item.Event
	if set.seen[key] {
		return
	}
	set.seen[key] = true
	set.list = append(set.list, item)
}

// Solve aligns all devices to the configured master device's local timeline.
func Solve(session Session) (*Report, error) {
	idx, diagnostics, err := indexSession(session)
	if err != nil {
		return nil, err
	}
	diag := &diagnosticSet{seen: make(map[string]bool)}
	for _, item := range diagnostics {
		diag.add(item)
	}
	states := buildSegmentStates(idx)

	for round := 0; round < 12; round++ {
		changed := false
		for _, state := range states {
			if state.status == "master" {
				continue
			}
			state.anchors = buildAnchors(state, idx, diag)
			state.fit = robustFit(state.anchors, idx.config)
			if state.fit != nil && state.fit.status == "locked" {
				state.status = "locked"
			} else if state.fit != nil {
				state.status = "provisional"
			} else {
				state.status = "unresolved"
			}
		}
		if propagateEvents(idx, states, diag) {
			changed = true
		}
		if !changed {
			break
		}
	}
	// One final fit after the last propagation round.
	for _, state := range states {
		if state.status == "master" {
			continue
		}
		state.anchors = buildAnchors(state, idx, diag)
		state.fit = robustFit(state.anchors, idx.config)
		state.status = "unresolved"
		if state.fit != nil {
			state.status = "provisional"
			if state.fit.status == "locked" {
				state.status = "locked"
			}
		}
	}

	report := &Report{
		Version:      1,
		MasterDevice: idx.masterID,
		Timeline:     "master device local time (seconds)",
		Config:       idx.config,
	}
	report.Events = buildEventInfos(idx)
	report.Devices = buildDeviceInfos(idx, states)
	sort.Slice(diag.list, func(i, j int) bool {
		a, b := diag.list[i], diag.list[j]
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Device != b.Device {
			return a.Device < b.Device
		}
		if a.Segment != b.Segment {
			return a.Segment < b.Segment
		}
		if a.Event != b.Event {
			return a.Event < b.Event
		}
		return a.Message < b.Message
	})
	report.Diagnostics = diag.list
	return report, nil
}

func buildSegmentStates(idx *indexedSession) []*segmentState {
	var states []*segmentState
	for i := range idx.session.Devices {
		device := &idx.session.Devices[i]
		for j := range device.Segments {
			state := &segmentState{device: device, segment: &device.Segments[j]}
			if device.ID == idx.masterID {
				state.status = "master"
			} else {
				state.status = "unresolved"
			}
			states = append(states, state)
		}
	}
	return states
}

func buildAnchors(state *segmentState, idx *indexedSession, diag *diagnosticSet) []anchor {
	byEvent := make(map[string][]Observation)
	for _, obs := range state.segment.Observations {
		byEvent[obs.Event] = append(byEvent[obs.Event], obs)
	}
	var anchors []anchor
	for eventID, observations := range byEvent {
		event := idx.events[eventID]
		if event.master == nil {
			continue
		}
		chosen, effective, ambiguous := selectObservation(observations, idx.config)
		if ambiguous {
			diag.add(Diagnostic{
				Severity: "warning", Code: "ambiguous_observation",
				Device: state.device.ID, Segment: state.segment.ID, Event: eventID,
				Message: "multiple similarly confident detections disagree; event excluded from this segment",
			})
			continue
		}
		if len(observations) > 1 {
			diag.add(Diagnostic{
				Severity: "info", Code: "duplicate_observation",
				Device: state.device.ID, Segment: state.segment.ID, Event: eventID,
				Message: "multiple detections present; highest-confidence detection used",
			})
		}
		anchors = append(anchors, anchor{
			event: eventID, kind: event.kind, local: chosen.Local,
			master: *event.master, confidence: effective * event.confidence,
		})
	}
	sort.Slice(anchors, func(i, j int) bool {
		if anchors[i].local == anchors[j].local {
			return anchors[i].event < anchors[j].event
		}
		return anchors[i].local < anchors[j].local
	})
	return anchors
}

func selectObservation(observations []Observation, cfg Config) (Observation, float64, bool) {
	if len(observations) == 1 {
		return observations[0], observationConfidence(observations[0]), false
	}
	sorted := append([]Observation(nil), observations...)
	sort.Slice(sorted, func(i, j int) bool {
		return observationConfidence(sorted[i]) > observationConfidence(sorted[j])
	})
	best := sorted[0]
	next := sorted[1]
	difference := math.Abs(best.Local-next.Local) * 1000
	if difference > cfg.AnchorToleranceMS*2 &&
		observationConfidence(next)+0.10 >= observationConfidence(best) {
		return best, 0, true
	}
	effective := observationConfidence(best)
	if difference > cfg.AnchorToleranceMS {
		effective *= 0.9
	}
	return best, effective, false
}
