package effectinterp

import (
	"math"
	"testing"
)

func TestInterpolateNumber(t *testing.T) {
	tests := []struct {
		name     string
		from     float64
		to       float64
		progress float64
		want     float64
	}{
		{name: "midpoint", from: 10, to: 20, progress: 0.25, want: 12.5},
		{name: "start", from: 10, to: 20, progress: 0, want: 10},
		{name: "end", from: 10, to: 20, progress: 1, want: 20},
		{name: "below range", from: 10, to: 20, progress: -1, want: 10},
		{name: "above range", from: 10, to: 20, progress: 2, want: 20},
		{name: "NaN", from: 10, to: 20, progress: math.NaN(), want: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InterpolateNumber(tt.from, tt.to, tt.progress); got != tt.want {
				t.Fatalf("InterpolateNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInterpolateAngle(t *testing.T) {
	tests := []struct {
		name     string
		from     float64
		to       float64
		progress float64
		want     float64
	}{
		{name: "forward across zero", from: 350, to: 10, progress: 0.5, want: 0},
		{name: "forward before zero", from: 350, to: 10, progress: 0.25, want: 355},
		{name: "backward across zero", from: 10, to: 350, progress: 0.25, want: 5},
		{name: "backward midpoint", from: 10, to: 350, progress: 0.5, want: 0},
		{name: "positive half turn", from: 0, to: 180, progress: 0.5, want: 90},
		{name: "negative half turn uses increasing direction", from: 180, to: 0, progress: 0.5, want: 270},
		{name: "normalize endpoint", from: 350, to: 370, progress: 1, want: 10},
		{name: "full turn is equivalent", from: 0, to: 720, progress: 0.5, want: 0},
		{name: "negative input", from: -10, to: 10, progress: 0.5, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InterpolateAngle(tt.from, tt.to, tt.progress)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("InterpolateAngle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInterpolateBooleanIsStepped(t *testing.T) {
	tests := []struct {
		progress float64
		want     bool
	}{
		{progress: 0, want: false},
		{progress: 0.999, want: false},
		{progress: 1, want: true},
		{progress: 2, want: true},
		{progress: math.NaN(), want: false},
	}
	for _, tt := range tests {
		if got := InterpolateBoolean(false, true, tt.progress); got != tt.want {
			t.Errorf("InterpolateBoolean(progress=%v) = %v, want %v", tt.progress, got, tt.want)
		}
	}
}

func TestInterpolateColor(t *testing.T) {
	from := Color{R: 0, G: 100, B: 200, A: 0}
	to := Color{R: 255, G: 200, B: 0, A: 255}
	want := Color{R: 128, G: 150, B: 100, A: 128}
	if got := InterpolateColor(from, to, 0.5); got != want {
		t.Fatalf("InterpolateColor() = %#v, want %#v", got, want)
	}
	if got := InterpolateColor(from, to, -1); got != from {
		t.Fatalf("below range = %#v, want %#v", got, from)
	}
	if got := InterpolateColor(from, to, 2); got != to {
		t.Fatalf("above range = %#v, want %#v", got, to)
	}
}

func TestNormalizeAngle(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{input: 0, want: 0},
		{input: 360, want: 0},
		{input: 721, want: 1},
		{input: -1, want: 359},
		{input: -360, want: 0},
	}
	for _, tt := range tests {
		if got := NormalizeAngle(tt.input); got != tt.want {
			t.Errorf("NormalizeAngle(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestInterpolateRejectsWrongKinds(t *testing.T) {
	_, err := Interpolate(
		KindNumber,
		NumberValue(0),
		AngleValue(10),
		0.5,
	)
	if err == nil {
		t.Fatal("expected a kind mismatch error")
	}
}
