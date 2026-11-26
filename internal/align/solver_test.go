package align

import (
	"fmt"
	"math"
	"testing"
)

func fp(value float64) *float64 {
	return &value
}

func obs(event string, local, confidence float64) Observation {
	return Observation{Event: event, Local: local, Confidence: fp(confidence)}
}

func masterDevice(id string, end float64, events []Event) Device {
	segment := Segment{ID: id + "-1", Start: 0, End: end}
	observations := make([]Observation, 0, len(events))
	for _, event := range events {
		if event.Kind != KindFlash {
			observations = append(observations, obs(event.ID, *event.MasterTime, 0.98))
		}
	}
	segment.Observations = observations
	return Device{ID: id, Role: "master", Segments: []Segment{segment}}
}

func clockEvents(prefix string, kind Kind, count int, start, step float64) []Event {
	events := make([]Event, 0, count)
	for i := 0; i < count; i++ {
		events = append(events, Event{
			ID: fmt.Sprintf("%s-%02d", prefix, i+1), Kind: kind,
			MasterTime: fp(start + float64(i)*step),
		})
	}
	return events
}

func localFor(master, slope, intercept float64) float64 {
	return (master - intercept) / slope
}

func cameraSegment(id string, start, end float64, events []Event, slope, intercept, confidence float64) Segment {
	segment := Segment{ID: id, Start: start, End: end}
	for _, event := range events {
		if event.MasterTime == nil || event.Kind == KindFlash {
			continue
		}
		local := localFor(*event.MasterTime, slope, intercept)
		if local >= start-1e-9 && local <= end+1e-9 {
			segment.Observations = append(segment.Observations, obs(event.ID, local, confidence))
		}
	}
	return segment
}

func findSegment(t *testing.T, report *Report, deviceID, segmentID string) SegmentReport {
	t.Helper()
	for _, device := range report.Devices {
		if device.ID != deviceID {
			continue
		}
		for _, segment := range device.Segments {
			if segment.ID == segmentID {
				return segment
			}
		}
	}
	t.Fatalf("segment %s/%s not found", deviceID, segmentID)
	return SegmentReport{}
}

func TestRobustFitRejectsFalseClapAndMeasuresDrift(t *testing.T) {
	claps := clockEvents("clap", KindClap, 10, 10, 60)
	all := append([]Event(nil), claps...)
	audio := masterDevice("audio", 600, all)
	segment := cameraSegment("cam-a-1", 0, 600, all, 1.0002, 0.4, 0.97)
	for i := range segment.Observations {
		if segment.Observations[i].Event == "clap-03" {
			segment.Observations[i] = obs("clap-03", 95, 0.99)
		}
	}
	session := Session{
		MasterDevice: "audio",
		Events:       all,
		Devices: []Device{audio, {
			ID: "cam-a", Role: "camera",
			Segments: []Segment{segment},
		}},
	}
	report, err := Solve(session)
	if err != nil {
		t.Fatal(err)
	}
	got := findSegment(t, report, "cam-a", "cam-a-1")
	if got.Status != "locked" {
		t.Fatalf("status = %s, want locked", got.Status)
	}
	if got.DriftPPM == nil || math.Abs(*got.DriftPPM-200) > 2 {
		t.Fatalf("drift = %v, want about 200 ppm", got.DriftPPM)
	}
	if !got.DriftConfirmed {
		t.Fatalf("drift should be confirmed")
	}
	if got.FirstTrusted == nil || got.FirstTrusted.Event != "clap-01" {
		t.Fatalf("first trusted = %+v, want clap-01", got.FirstTrusted)
	}
	if got.Fit == nil || got.Fit.RejectedAnchors < 1 {
		t.Fatalf("fit did not reject false clap: %+v", got.Fit)
	}
	if len(got.Map) == 0 {
		t.Fatalf("locked segment emitted no mapping")
	}
}

func TestCameraRestartGetsIndependentOffsetAndDrift(t *testing.T) {
	events := clockEvents("clap", KindClap, 10, 10, 60)
	audio := masterDevice("audio", 600, events)
	first := cameraSegment("cam-b-1", 0, 300, events, 1.00005, 0.2, 0.97)
	second := cameraSegment("cam-b-2", 0, 300, events, 0.9998, 300.05, 0.97)
	session := Session{
		MasterDevice: "audio",
		Events:       events,
		Devices: []Device{audio, {
			ID: "cam-b", Role: "camera",
			Segments: []Segment{first, second},
		}},
	}
	report, err := Solve(session)
	if err != nil {
		t.Fatal(err)
	}
	one := findSegment(t, report, "cam-b", "cam-b-1")
	two := findSegment(t, report, "cam-b", "cam-b-2")
	for _, segment := range []SegmentReport{one, two} {
		if segment.Status != "locked" || segment.FirstTrusted == nil {
			t.Fatalf("restarted segment not independently locked: %+v", segment)
		}
	}
	if one.OffsetAtStart == nil || math.Abs(*one.OffsetAtStart-0.2) > 0.01 {
		t.Fatalf("segment one offset = %v", one.OffsetAtStart)
	}
	if two.OffsetAtStart == nil || math.Abs(*two.OffsetAtStart-300.05) > 0.02 {
		t.Fatalf("segment two offset = %v", two.OffsetAtStart)
	}
	if one.DriftPPM == nil || math.Abs(*one.DriftPPM-50) > 1 {
		t.Fatalf("segment one drift = %v", one.DriftPPM)
	}
	if two.DriftPPM == nil || math.Abs(*two.DriftPPM+200) > 1 {
		t.Fatalf("segment two drift = %v", two.DriftPPM)
	}
}
