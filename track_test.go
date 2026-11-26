package effectinterp

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestTrackNumberInterpolationAndHold(t *testing.T) {
	track := mustTrack(t, KindNumber, []Keyframe{
		{Time: 2 * time.Second, Value: NumberValue(20)},
		{Time: 0, Value: NumberValue(10)},
	})

	tests := []struct {
		at   time.Duration
		want float64
	}{
		{at: -time.Second, want: 10},
		{at: 0, want: 10},
		{at: time.Second, want: 15},
		{at: 2 * time.Second, want: 20},
		{at: 3 * time.Second, want: 20},
	}
	for _, tt := range tests {
		value := mustValueAt(t, track, tt.at)
		if value.Number != tt.want {
			t.Errorf("At(%v) = %v, want %v", tt.at, value.Number, tt.want)
		}
	}
}

func TestTrackDuplicateTimestampUsesLastValue(t *testing.T) {
	track := mustTrack(t, KindNumber, []Keyframe{
		{Time: 10 * time.Second, Value: NumberValue(10)},
		{Time: 0, Value: NumberValue(0)},
		{Time: 10 * time.Second, Value: NumberValue(20)},
		{Time: 20 * time.Second, Value: NumberValue(40)},
	})

	keys := track.Keyframes()
	wantKeys := []Keyframe{
		{Time: 0, Value: NumberValue(0)},
		{Time: 10 * time.Second, Value: NumberValue(20)},
		{Time: 20 * time.Second, Value: NumberValue(40)},
	}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("Keyframes() = %#v, want %#v", keys, wantKeys)
	}

	if got := mustValueAt(t, track, 5*time.Second).Number; got != 10 {
		t.Errorf("before duplicate = %v, want 10", got)
	}
	if got := mustValueAt(t, track, 10*time.Second).Number; got != 20 {
		t.Errorf("at duplicate = %v, want last value 20", got)
	}
	if got := mustValueAt(t, track, 15*time.Second).Number; got != 30 {
		t.Errorf("after duplicate = %v, want 30", got)
	}
}

func TestTrackAllDuplicatesBecomeConstant(t *testing.T) {
	track := mustTrack(t, KindColor, []Keyframe{
		{Time: time.Second, Value: RGBA(1, 2, 3, 4)},
		{Time: time.Second, Value: RGBA(5, 6, 7, 8)},
	})

	if got := mustValueAt(t, track, 0); got.Color != (Color{R: 5, G: 6, B: 7, A: 8}) {
		t.Errorf("before = %#v", got.Color)
	}
	if got := mustValueAt(t, track, 2*time.Second); got.Color != (Color{R: 5, G: 6, B: 7, A: 8}) {
		t.Errorf("after = %#v", got.Color)
	}
}

func TestTrackBooleanStepsAtRightKeyframe(t *testing.T) {
	track := mustTrack(t, KindBoolean, []Keyframe{
		{Time: 0, Value: BooleanValue(false)},
		{Time: time.Second, Value: BooleanValue(true)},
	})

	if got := mustValueAt(t, track, 999*time.Millisecond).Boolean; got {
		t.Error("boolean changed before its right keyframe")
	}
	if got := mustValueAt(t, track, time.Second).Boolean; !got {
		t.Error("boolean did not change at its right keyframe")
	}
}

func TestTrackAngleCrossesZero(t *testing.T) {
	track := mustTrack(t, KindAngle, []Keyframe{
		{Time: 0, Value: AngleValue(350)},
		{Time: time.Second, Value: AngleValue(370)},
	})

	if got := mustValueAt(t, track, 500*time.Millisecond).Number; math.Abs(got) > 1e-9 {
		t.Fatalf("midpoint angle = %v, want 0", got)
	}
	if got := mustValueAt(t, track, time.Second).Number; got != 10 {
		t.Fatalf("endpoint angle = %v, want normalized 10", got)
	}
}

func TestNewTrackDoesNotMutateInput(t *testing.T) {
	input := []Keyframe{
		{Time: 2 * time.Second, Value: NumberValue(2)},
		{Time: time.Second, Value: NumberValue(1)},
	}
	want := append([]Keyframe(nil), input...)
	mustTrack(t, KindNumber, input)
	if !reflect.DeepEqual(input, want) {
		t.Fatalf("NewTrack mutated input: %#v", input)
	}
}

func TestNewTrackValidation(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		keys []Keyframe
	}{
		{name: "empty", kind: KindNumber},
		{name: "unknown kind", kind: Kind(99), keys: []Keyframe{{Value: NumberValue(1)}}},
		{
			name: "wrong value kind",
			kind: KindNumber,
			keys: []Keyframe{{Value: AngleValue(1)}},
		},
		{
			name: "non-finite number",
			kind: KindNumber,
			keys: []Keyframe{{Value: NumberValue(math.NaN())}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewTrack(tt.kind, tt.keys); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestZeroTrackReturnsError(t *testing.T) {
	var track Track
	if _, err := track.At(0); err == nil {
		t.Fatal("expected an error from an uninitialized track")
	}
}

func mustTrack(t *testing.T, kind Kind, keys []Keyframe) Track {
	t.Helper()
	track, err := NewTrack(kind, keys)
	if err != nil {
		t.Fatalf("NewTrack() error = %v", err)
	}
	return track
}

func mustValueAt(t *testing.T, track Track, at time.Duration) Value {
	t.Helper()
	value, err := track.At(at)
	if err != nil {
		t.Fatalf("At(%v) error = %v", at, err)
	}
	return value
}
