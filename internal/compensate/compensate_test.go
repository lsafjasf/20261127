package compensate

import (
	"math"
	"testing"

	"blacklift/internal/frameio"
)

func makeFrame(w, h int, v float64) *frameio.Frame {
	n := w * h
	fr := &frameio.Frame{Width: w, Height: h, R: make([]float64, n), G: make([]float64, n), B: make([]float64, n), Y: make([]float64, n)}
	for i := 0; i < n; i++ {
		fr.R[i], fr.G[i], fr.B[i] = v, v, v
		fr.Y[i] = v
	}
	return fr
}

func TestNoiseFloorAndCapBoundCompensation(t *testing.T) {
	o := DefaultOptions()
	sigma := 6.0 / 255.0
	// Raw fog 20/255: noise floor b-k*sigma = 20-9 = 11, under the 16 cap.
	rawLevel := 20.0 / 255.0
	w, h := 16, 16
	fr := makeFrame(w, h, 30.0/255.0)
	raw := make([]float64, w*h)
	for i := range raw {
		raw[i] = rawLevel
	}
	res := Apply(fr, raw, sigma, false, o)
	want := rawLevel - o.NoiseK*sigma
	if math.Abs(res.AppliedMean-want) > 0.01/255.0 {
		t.Fatalf("applied %.3f want %.3f", res.AppliedMean, want)
	}
	if res.AppliedMax > o.MaxComp+1e-9 {
		t.Fatal("hard cap violated")
	}

	// Huge fog must be clamped at MaxComp, never subtracting the full value.
	for i := range raw {
		raw[i] = 80.0 / 255.0
	}
	fr2 := makeFrame(w, h, 90.0/255.0)
	res2 := Apply(fr2, raw, sigma, false, o)
	if res2.AppliedMax > o.MaxComp+1e-9 {
		t.Fatalf("cap violated: %.4f", res2.AppliedMax)
	}
}

func TestStyleLiftScalesToZero(t *testing.T) {
	o := DefaultOptions()
	w, h := 8, 8
	fr := makeFrame(w, h, 40.0/255.0)
	raw := make([]float64, w*h)
	for i := range raw {
		raw[i] = 15.0 / 255.0
	}
	before := fr.Y[0]
	res := Apply(fr, raw, 1.0/255.0, true, o)
	if res.AppliedMax != 0 {
		t.Fatalf("style shot must be preserved, applied %.4f", res.AppliedMax)
	}
	if math.Abs(fr.Y[0]-before) > 1e-12 {
		t.Fatal("style frame pixels were modified")
	}
}

func TestBlackGuardPreventsNewZero(t *testing.T) {
	o := DefaultOptions()
	w, h := 8, 8
	// Pixels sit just above the guard; subtraction is limited so they survive.
	fr := makeFrame(w, h, 3.0/255.0)
	raw := make([]float64, w*h)
	for i := range raw {
		raw[i] = 10.0 / 255.0
	}
	res := Apply(fr, raw, 0.5/255.0, false, o)
	if res.NewClippedLuma != 0 {
		t.Fatalf("guard should prevent new clipping, got %d px", res.NewClippedLuma)
	}
	if fr.Y[0] < o.BlackGuard-1e-12 {
		t.Fatalf("pixel driven below guard: %.4f", fr.Y[0])
	}
}

func TestClippingIsCountedAndCapped(t *testing.T) {
	o := DefaultOptions()
	o.BlackGuard = 0 // allow clipping so the metric is exercised
	w, h := 8, 8
	fr := makeFrame(w, h, 4.0/255.0)
	raw := make([]float64, w*h)
	for i := range raw {
		raw[i] = 30.0 / 255.0 // bounded to MaxComp=16/255, still clips 4/255 pixels
	}
	res := Apply(fr, raw, 0.2/255.0, false, o)
	if res.NewClippedLuma != int64(w*h) {
		t.Fatalf("all pixels should newly clip, got %d", res.NewClippedLuma)
	}
	if res.AppliedMax > o.MaxComp+1e-9 {
		t.Fatal("cap violated")
	}
	if fr.Y[0] != 0 {
		t.Fatal("expected clipped zero")
	}
}

func TestSmoothMapsZeroPhase(t *testing.T) {
	maps := [][]float64{
		make([]float64, 4),
		{10, 10, 10, 10},
		{10, 10, 10, 10},
	}
	SmoothMaps(maps, 0.5)
	// first and last frames should both move toward the steady state 10.
	if maps[0][0] <= 0 || maps[2][0] <= 0 {
		t.Fatal("smoothing should propagate bidirectionally")
	}
}
