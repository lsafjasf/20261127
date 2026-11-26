package align

import (
	"math"
	"strings"
	"testing"
)

func TestFlashPropagatesThroughLockedCamera(t *testing.T) {
	claps := clockEvents("clap", KindClap, 5, 10, 50)
	flashes := []Event{
		{ID: "flash-01", Kind: KindFlash},
		{ID: "flash-02", Kind: KindFlash},
		{ID: "flash-03", Kind: KindFlash},
	}
	expectedFlashMaster := []float64{100, 160, 220}
	all := append(append([]Event(nil), claps...), flashes...)
	audio := masterDevice("audio", 300, all)

	camA := cameraSegment("cam-a-1", 0, 300, claps, 1.0001, 0.2, 0.97)
	camB := Segment{ID: "cam-b-1", Start: 0, End: 300}
	for i, masterTime := range expectedFlashMaster {
		camA.Observations = append(camA.Observations,
			obs(flashes[i].ID, localFor(masterTime, 1.0001, 0.2), 0.96))
		camB.Observations = append(camB.Observations,
			obs(flashes[i].ID, localFor(masterTime, 0.99995, 0.7), 0.96))
	}
	session := Session{
		MasterDevice: "audio", Events: all,
		Devices: []Device{
			audio,
			{ID: "cam-a", Role: "camera", Segments: []Segment{camA}},
			{ID: "cam-b", Role: "camera", Segments: []Segment{camB}},
		},
	}
	report, err := Solve(session)
	if err != nil {
		t.Fatal(err)
	}
	for _, eventID := range []string{"flash-01", "flash-02", "flash-03"} {
		var info EventInfo
		for _, candidate := range report.Events {
			if candidate.ID == eventID {
				info = candidate
			}
		}
		if info.Source != "propagated" || info.MasterTime == nil {
			t.Fatalf("%s not propagated: %+v", eventID, info)
		}
	}
	gotA := findSegment(t, report, "cam-a", "cam-a-1")
	gotB := findSegment(t, report, "cam-b", "cam-b-1")
	if gotA.Status != "locked" || gotB.Status != "locked" {
		t.Fatalf("camera statuses = %s, %s", gotA.Status, gotB.Status)
	}
	if gotB.FirstTrusted == nil || gotB.FirstTrusted.Kind != KindFlash {
		t.Fatalf("camera B should lock on a flash: %+v", gotB.FirstTrusted)
	}
	if gotB.DriftPPM == nil || math.Abs(*gotB.DriftPPM+50) > 1 {
		t.Fatalf("camera B drift = %v, want about -50 ppm", gotB.DriftPPM)
	}
}

func TestUnresolvedProvisionalAndInconsistentSegments(t *testing.T) {
	events := []Event{
		{ID: "clap-1", Kind: KindClap, MasterTime: fp(10)},
		{ID: "clap-2", Kind: KindClap, MasterTime: fp(20)},
	}
	audio := masterDevice("audio", 120, events)
	camC := Segment{ID: "cam-c-1", Start: 0, End: 60}
	camD := Segment{
		ID: "cam-d-1", Start: 0, End: 60,
		Observations: []Observation{obs("clap-1", 10, 0.99)},
	}
	camE := Segment{
		ID: "cam-e-1", Start: 0, End: 60,
		Observations: []Observation{
			obs("clap-1", 10, 0.98),
			obs("clap-2", 12, 0.98),
		},
	}
	session := Session{
		MasterDevice: "audio", Events: events,
		Devices: []Device{
			audio,
			{ID: "cam-c", Segments: []Segment{camC}},
			{ID: "cam-d", Segments: []Segment{camD}},
			{ID: "cam-e", Segments: []Segment{camE}},
		},
	}
	report, err := Solve(session)
	if err != nil {
		t.Fatal(err)
	}
	gotC := findSegment(t, report, "cam-c", "cam-c-1")
	if gotC.Status != "unresolved" || len(gotC.Indeterminate) != 1 ||
		gotC.Indeterminate[0].Reason != "no_usable_anchors" {
		t.Fatalf("empty segment = %+v", gotC)
	}
	gotD := findSegment(t, report, "cam-d", "cam-d-1")
	if gotD.Status != "provisional" || gotD.FirstTrusted != nil {
		t.Fatalf("single-anchor segment = %+v", gotD)
	}
	if len(gotD.Map) == 0 || len(gotD.Indeterminate) == 0 {
		t.Fatalf("provisional segment lacks bounded map/unknown split: %+v", gotD)
	}
	gotE := findSegment(t, report, "cam-e", "cam-e-1")
	if gotE.Status != "unresolved" || len(gotE.Indeterminate) != 1 ||
		gotE.Indeterminate[0].Reason != "inconsistent_anchors" {
		t.Fatalf("inconsistent segment = %+v", gotE)
	}
}

func TestMasterAudioObservationsSeedEventTimes(t *testing.T) {
	events := []Event{
		{ID: "clap-1", Kind: KindClap},
		{ID: "clap-2", Kind: KindClap},
	}
	audio := Device{
		ID: "audio",
		Segments: []Segment{{
			ID: "audio-1", Start: 0, End: 100,
			Observations: []Observation{
				obs("clap-1", 10, 0.99),
				obs("clap-2", 70, 0.99),
			},
		}},
	}
	camera := Device{
		ID: "camera",
		Segments: []Segment{{
			ID: "camera-1", Start: 0, End: 100,
			Observations: []Observation{
				obs("clap-1", 9.8, 0.97),
				obs("clap-2", 69.8, 0.97),
			},
		}},
	}
	report, err := Solve(Session{
		MasterDevice: "audio", Events: events,
		Devices: []Device{audio, camera},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range report.Events {
		if event.Source != "master_observation" || event.MasterTime == nil {
			t.Fatalf("event %s = %+v", event.ID, event)
		}
	}
	got := findSegment(t, report, "camera", "camera-1")
	if got.Status != "locked" || got.OffsetAtStart == nil ||
		math.Abs(*got.OffsetAtStart-0.2) > 0.001 {
		t.Fatalf("camera alignment = %+v", got)
	}
}

func TestSingleAnchorCanBeTrustedByConfig(t *testing.T) {
	events := []Event{{ID: "clap-1", Kind: KindClap, MasterTime: fp(10)}}
	session := Session{
		MasterDevice: "audio", Events: events,
		Config: Config{MinOffsetAnchors: 1},
		Devices: []Device{
			masterDevice("audio", 60, events),
			{ID: "camera", Segments: []Segment{{
				ID: "camera-1", Start: 0, End: 60,
				Observations: []Observation{obs("clap-1", 10.2, 0.99)},
			}}},
		},
	}
	report, err := Solve(session)
	if err != nil {
		t.Fatal(err)
	}
	got := findSegment(t, report, "camera", "camera-1")
	if got.Status != "locked" || got.FirstTrusted == nil {
		t.Fatalf("single-anchor override = %+v", got)
	}
}

func TestValidationRejectsUnsafeManifest(t *testing.T) {
	tests := []struct {
		name    string
		session Session
		want    string
	}{
		{
			name:    "missing master",
			session: Session{Devices: []Device{{ID: "cam", Segments: []Segment{{ID: "s", Start: 0, End: 1}}}}},
			want:    "master_device",
		},
		{
			name: "observation outside segment",
			session: Session{
				MasterDevice: "audio",
				Events:       []Event{{ID: "e", Kind: KindClap, MasterTime: fp(1)}},
				Devices: []Device{{
					ID: "audio", Segments: []Segment{{
						ID: "s", Start: 0, End: 1,
						Observations: []Observation{obs("e", 2, 0.9)},
					}},
				}},
			},
			want: "outside",
		},
		{
			name: "duplicate event",
			session: Session{
				MasterDevice: "audio",
				Events:       []Event{{ID: "e", Kind: KindClap}, {ID: "e", Kind: KindClap}},
				Devices:      []Device{masterDevice("audio", 1, nil)},
			},
			want: "duplicate event",
		},
		{
			name: "overlapping master segments",
			session: Session{
				MasterDevice: "audio",
				Devices: []Device{{
					ID: "audio",
					Segments: []Segment{
						{ID: "a", Start: 0, End: 10},
						{ID: "b", Start: 5, End: 15},
					},
				}},
			},
			want: "overlap",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Solve(test.session)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}
