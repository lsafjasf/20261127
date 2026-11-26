package effectinterp

import (
	"reflect"
	"testing"
	"time"
)

func TestPreviewAndOfflineFramesMatch(t *testing.T) {
	scene := testScene(t)
	at := 500 * time.Millisecond

	preview, err := scene.PreviewFrame(at)
	if err != nil {
		t.Fatalf("PreviewFrame() error = %v", err)
	}
	offline, err := scene.RenderFrame(at)
	if err != nil {
		t.Fatalf("RenderFrame() error = %v", err)
	}
	if !reflect.DeepEqual(preview, offline) {
		t.Fatalf("preview = %#v, offline = %#v", preview, offline)
	}

	want := Frame{
		"angle":   AngleValue(0),
		"color":   ColorValue(Color{R: 128, G: 0, B: 0, A: 128}),
		"enabled": BooleanValue(false),
		"opacity": NumberValue(0.5),
	}
	if !reflect.DeepEqual(preview, want) {
		t.Fatalf("frame = %#v, want %#v", preview, want)
	}
}

func TestRenderRangeUsesSameSamplesAsPreview(t *testing.T) {
	scene := testScene(t)
	frames, err := scene.RenderRange(0, time.Second, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("RenderRange() error = %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("len(frames) = %d, want 5", len(frames))
	}

	for i, frame := range frames {
		at := time.Duration(i) * 250 * time.Millisecond
		preview, err := scene.PreviewFrame(at)
		if err != nil {
			t.Fatalf("PreviewFrame(%v) error = %v", at, err)
		}
		if !reflect.DeepEqual(frame, preview) {
			t.Errorf("frame %d = %#v, preview = %#v", i, frame, preview)
		}
	}
}

func TestSceneParameterNamesAreSorted(t *testing.T) {
	scene := testScene(t)
	want := []string{"angle", "color", "enabled", "opacity"}
	if got := scene.ParameterNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ParameterNames() = %v, want %v", got, want)
	}
}

func TestNewSceneValidation(t *testing.T) {
	valid := mustTrack(t, KindNumber, []Keyframe{{Value: NumberValue(1)}})
	tests := []struct {
		name   string
		tracks map[string]Track
	}{
		{name: "empty name", tracks: map[string]Track{"": valid}},
		{name: "zero track", tracks: map[string]Track{"opacity": {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewScene(tt.tracks); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestRenderRangeValidation(t *testing.T) {
	scene := testScene(t)
	tests := []struct {
		name       string
		start, end time.Duration
		step       time.Duration
	}{
		{name: "zero step", start: 0, end: time.Second, step: 0},
		{name: "negative step", start: 0, end: time.Second, step: -time.Second},
		{name: "end before start", start: time.Second, end: 0, step: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := scene.RenderRange(tt.start, tt.end, tt.step); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func testScene(t *testing.T) *Scene {
	t.Helper()
	tracks := map[string]Track{
		"opacity": mustTrack(t, KindNumber, []Keyframe{
			{Time: 0, Value: NumberValue(0)},
			{Time: time.Second, Value: NumberValue(1)},
		}),
		"angle": mustTrack(t, KindAngle, []Keyframe{
			{Time: 0, Value: AngleValue(350)},
			{Time: time.Second, Value: AngleValue(370)},
		}),
		"enabled": mustTrack(t, KindBoolean, []Keyframe{
			{Time: 0, Value: BooleanValue(false)},
			{Time: time.Second, Value: BooleanValue(true)},
		}),
		"color": mustTrack(t, KindColor, []Keyframe{
			{Time: 0, Value: RGBA(0, 0, 0, 0)},
			{Time: time.Second, Value: RGBA(255, 0, 0, 255)},
		}),
	}
	scene, err := NewScene(tracks)
	if err != nil {
		t.Fatalf("NewScene() error = %v", err)
	}
	return scene
}
