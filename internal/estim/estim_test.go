package estim

import (
	"math"
	"testing"
)

func TestNeighborSigmaRecoversNoise(t *testing.T) {
	const w, h = 64, 64
	y := make([]float64, w*h)
	want := 6.0 / 255.0
	// deterministic pseudo-noise
	seed := uint64(12345)
	next := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		u := float64(seed>>11)/(1<<53) - 0.5
		return u * 3.46 // approx unit-scale uniform-ish
	}
	for i := range y {
		y[i] = 0.2 + next()*want
	}
	g := BuildGrid(w, h, 4, 4, 8)
	g.Stats(y)
	got := g.FrameSigma()
	if math.Abs(got-want) > want*0.35 {
		t.Fatalf("sigma estimate %.4f not within 35%% of %.4f", got, want)
	}
}

func TestFitMaskRecoversRadialLift(t *testing.T) {
	const w, h = 128, 72
	y := make([]float64, w*h)
	base, corner := 12.0/255.0, 7.0/255.0
	for row := 0; row < h; row++ {
		ny := (float64(row)+0.5)/float64(h)*2 - 1
		for col := 0; col < w; col++ {
			nx := (float64(col)+0.5)/float64(w)*2 - 1
			rr := (nx*nx + ny*ny) / 2 // rho^2 with corners at rho=1
			y[row*w+col] = base + corner*rr
		}
	}
	g := BuildGrid(w, h, 8, 5, 12)
	g.Stats(y)
	m := FitMask(y, w, h, 24)
	if math.Abs(m.Base-base) > 1.5/255.0 {
		t.Fatalf("base %.4f vs want %.4f", m.Base, base)
	}
	if math.Abs(m.Corner-(base+corner)) > 1.5/255.0 {
		t.Fatalf("corner %.4f vs want %.4f", m.Corner, base+corner)
	}
	if m.RSquared < 0.95 {
		t.Fatalf("r2 too low: %.3f", m.RSquared)
	}
}

func TestPercentile(t *testing.T) {
	v := []float64{1, 2, 3, 4}
	if got := percentile(v, 0); got != 1 {
		t.Fatal("p0", got)
	}
	if got := percentile(v, 1); got != 4 {
		t.Fatal("p100", got)
	}
	if got := percentile(v, 0.5); math.Abs(got-2.5) > 1e-9 {
		t.Fatal("p50", got)
	}
}
