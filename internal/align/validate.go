package align

import (
	"fmt"
	"math"
	"sort"
)

type eventState struct {
	kind       Kind
	master     *float64
	source     string
	confidence float64
	observedBy map[string]bool
}

type indexedSession struct {
	session    Session
	config     Config
	events     map[string]*eventState
	masterID   string
	segmentIDs map[string]bool
}

func indexSession(session Session) (*indexedSession, []Diagnostic, error) {
	cfg, err := session.Config.Normalize()
	if err != nil {
		return nil, nil, err
	}
	if session.MasterDevice == "" {
		return nil, nil, fmt.Errorf("master_device is required")
	}
	if len(session.Devices) == 0 {
		return nil, nil, fmt.Errorf("devices is empty")
	}

	idx := &indexedSession{
		session:    session,
		config:     cfg,
		events:     make(map[string]*eventState, len(session.Events)),
		masterID:   session.MasterDevice,
		segmentIDs: make(map[string]bool),
	}
	var diagnostics []Diagnostic

	for i := range session.Events {
		event := &session.Events[i]
		if event.ID == "" {
			return nil, nil, fmt.Errorf("events[%d].id is required", i)
		}
		if _, exists := idx.events[event.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate event id %q", event.ID)
		}
		if event.Kind == "" {
			event.Kind = KindMarker
		}
		if !validKind(event.Kind) {
			return nil, nil, fmt.Errorf("event %q has unknown kind %q", event.ID, event.Kind)
		}
		if event.MasterTime != nil {
			if !finite(*event.MasterTime) {
				return nil, nil, fmt.Errorf("event %q has non-finite master_time", event.ID)
			}
			value := *event.MasterTime
			idx.events[event.ID] = &eventState{
				kind: event.Kind, master: &value,
				source: "explicit", confidence: 1, observedBy: make(map[string]bool),
			}
		} else {
			idx.events[event.ID] = &eventState{
				kind: event.Kind, observedBy: make(map[string]bool),
			}
		}
	}

	deviceIDs := make(map[string]bool, len(session.Devices))
	var masterSpans []span
	for i := range session.Devices {
		device := &session.Devices[i]
		if device.ID == "" {
			return nil, nil, fmt.Errorf("devices[%d].id is required", i)
		}
		if deviceIDs[device.ID] {
			return nil, nil, fmt.Errorf("duplicate device id %q", device.ID)
		}
		deviceIDs[device.ID] = true
		if len(device.Segments) == 0 {
			return nil, nil, fmt.Errorf("device %q has no segments", device.ID)
		}
		if device.FPS < 0 || math.IsNaN(device.FPS) || math.IsInf(device.FPS, 0) {
			return nil, nil, fmt.Errorf("device %q has invalid fps", device.ID)
		}
		for j := range device.Segments {
			segment := &device.Segments[j]
			if segment.ID == "" {
				return nil, nil, fmt.Errorf("device %q segments[%d].id is required", device.ID, j)
			}
			if idx.segmentIDs[segment.ID] {
				return nil, nil, fmt.Errorf("duplicate segment id %q", segment.ID)
			}
			idx.segmentIDs[segment.ID] = true
			if !finite(segment.Start) || !finite(segment.End) || segment.End <= segment.Start {
				return nil, nil, fmt.Errorf("segment %q must have finite start < end", segment.ID)
			}
			if device.ID == idx.masterID {
				masterSpans = append(masterSpans, span{start: segment.Start, end: segment.End})
			}
			for k := range segment.Observations {
				obs := &segment.Observations[k]
				if _, ok := idx.events[obs.Event]; !ok {
					return nil, nil, fmt.Errorf("segment %q references unknown event %q", segment.ID, obs.Event)
				}
				if !finite(obs.Local) {
					return nil, nil, fmt.Errorf("segment %q has non-finite observation time", segment.ID)
				}
				if obs.Local < segment.Start-1e-9 || obs.Local > segment.End+1e-9 {
					return nil, nil, fmt.Errorf("segment %q observation %q at %.6f is outside [%.6f,%.6f]",
						segment.ID, obs.Event, obs.Local, segment.Start, segment.End)
				}
				if obs.Confidence != nil && (*obs.Confidence < 0 || *obs.Confidence > 1 ||
					math.IsNaN(*obs.Confidence) || math.IsInf(*obs.Confidence, 0)) {
					return nil, nil, fmt.Errorf("segment %q observation %q has invalid confidence", segment.ID, obs.Event)
				}
				idx.events[obs.Event].observedBy[device.ID] = true
			}
		}
	}
	if !deviceIDs[idx.masterID] {
		return nil, nil, fmt.Errorf("master_device %q does not exist", idx.masterID)
	}
	sort.Slice(masterSpans, func(i, j int) bool { return masterSpans[i].start < masterSpans[j].start })
	for i := 1; i < len(masterSpans); i++ {
		if masterSpans[i].start < masterSpans[i-1].end-1e-9 {
			return nil, nil, fmt.Errorf("master device segments overlap in local time")
		}
	}

	diagnostics = append(diagnostics, seedMasterEvents(idx)...)
	return idx, diagnostics, nil
}

func seedMasterEvents(idx *indexedSession) []Diagnostic {
	var diagnostics []Diagnostic
	eventsByMaster := make(map[string][]Observation)
	for _, device := range idx.session.Devices {
		if device.ID != idx.masterID {
			continue
		}
		for _, segment := range device.Segments {
			for _, obs := range segment.Observations {
				eventsByMaster[obs.Event] = append(eventsByMaster[obs.Event], obs)
			}
		}
	}

	for eventID, observations := range eventsByMaster {
		state := idx.events[eventID]
		best := observations[0]
		bestConfidence := observationConfidence(best)
		for _, obs := range observations[1:] {
			if confidence := observationConfidence(obs); confidence > bestConfidence {
				best, bestConfidence = obs, confidence
			}
		}
		if state.master != nil {
			if math.Abs(*state.master-best.Local)*1000 > idx.config.AnchorToleranceMS*2 {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: "warning", Code: "master_time_conflict", Event: eventID,
					Message: fmt.Sprintf("explicit master_time %.6f conflicts with master observation %.6f",
						*state.master, best.Local),
				})
			}
			continue
		}
		value := best.Local
		state.master = &value
		state.source = "master_observation"
		state.confidence = bestConfidence * kindFactor(state.kind, idx.config)
	}
	return diagnostics
}

func observationConfidence(obs Observation) float64 {
	if obs.Confidence == nil {
		return 1
	}
	return *obs.Confidence
}

func kindFactor(kind Kind, config Config) float64 {
	if kind == KindAmbient {
		return config.AmbientConfidenceFactor
	}
	return 1
}

func validKind(kind Kind) bool {
	switch kind {
	case KindClap, KindFlash, KindAmbient, KindMarker:
		return true
	default:
		return false
	}
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
