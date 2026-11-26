package flatfield

import (
	"math"
	"path/filepath"
	"testing"
)

const testW = 160
const testH = 120

type syntheticField struct {
	aspect float64
	rScale float64
}

func newSyntheticField() syntheticField {
	aspect := float64(testW) / float64(testH)
	rScale := math.Max(
		math.Hypot(0.5*aspect, 0.5),
		math.Hypot(-0.5*aspect, 0.5),
	)
	return syntheticField{aspect: aspect, rScale: rScale}
}

func (s syntheticField) coords(x, y int) (u, v, dx, dy, r, r2 float64) {
	u = (float64(x) + .5) / testW
	v = (float64(y) + .5) / testH
	dx = (u - .5) * s.aspect
	dy = v - .5
	r = math.Hypot(dx, dy) / s.rScale
	r2 = r * r
	return
}

func (s syntheticField) lensLog(r2 float64) float64 {
	return -0.30*r2 - 0.12*r2*r2
}

func (s syntheticField) gradientLog(dx, dy float64) float64 {
	return 0.10*dx - 0.16*dy
}

func (s syntheticField) panelLog(u, v float64) float64 {
	return 0.18 * (2*u - 1) * (2*v - 1)
}

func (s syntheticField) totalLog(x, y int) float64 {
	u, v, dx, dy, _, r2 := s.coords(x, y)
	return s.lensLog(r2) + s.gradientLog(dx, dy) + s.panelLog(u, v)
}

func testFlatRaster(s syntheticField) Raster {
	base := RGB{R: 0.72, G: 0.56, B: 0.42}
	return FuncRaster{W: testW, H: testH, Fn: func(x, y int) RGB {
		gain := math.Exp(s.totalLog(x, y))
		return RGB{R: base.R * gain, G: base.G * gain, B: base.B * gain}
	}}
}

func testOptions() Options {
	o := DefaultOptions()
	o.SmoothSigma = 0.25
	o.MaxGain = -1
	return o
}

func TestEstimateDecomposesFlatField(t *testing.T) {
	s := newSyntheticField()
	tab, err := Estimate(testFlatRaster(s), nil, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := tab.Validate(); err != nil {
		t.Fatal(err)
	}
	if !tab.Report.Gradient.Significant {
		t.Fatalf("gradient not detected: %+v", tab.Report.Gradient)
	}
	if math.Abs(tab.Report.Gradient.LogSlopeX-0.10) > 0.015 {
		t.Fatalf("slope x = %.4f, want ~0.10", tab.Report.Gradient.LogSlopeX)
	}
	if math.Abs(tab.Report.Gradient.LogSlopeY+0.16) > 0.015 {
		t.Fatalf("slope y = %.4f, want ~-0.16", tab.Report.Gradient.LogSlopeY)
	}
	if math.Abs(tab.Report.Lens.Coefficients[0]+0.30) > 0.025 {
		t.Fatalf("r2 coefficient = %.4f, want ~-0.30", tab.Report.Lens.Coefficients[0])
	}
	if math.Abs(tab.Report.Lens.Coefficients[1]+0.12) > 0.03 {
		t.Fatalf("r4 coefficient = %.4f, want ~-0.12", tab.Report.Lens.Coefficients[1])
	}
	if tab.Report.Identifiability == "" || tab.Report.Confidence <= 0 || tab.Report.Confidence > 1 {
		t.Fatalf("bad identifiability report: %+v", tab.Report)
	}

	for _, u := range []float64{0.02, 0.25, 0.5, 0.75, 0.98} {
		for _, v := range []float64{0.02, 0.5, 0.98} {
			got := tab.ResponseAt(u, v)
			want, err := tab.ComponentResponseAt(KindDevice, u, v)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-want)/want > 2e-4 {
				t.Fatalf("primary response not device at %.2f,%.2f: got %.6f want %.6f", u, v, got, want)
			}
		}
	}
}

func TestDeviceGainCorrectsAndPreservesColorRatios(t *testing.T) {
	s := newSyntheticField()
	opts := testOptions()
	opts.Kind = KindDeviceNoGradient
	tab, err := Estimate(testFlatRaster(s), nil, opts)
	if err != nil {
		t.Fatal(err)
	}

	const w, h = 81, 61
	frame, err := NewFrame(w, h)
	if err != nil {
		t.Fatal(err)
	}
	base := RGB{R: 0.64, G: 0.48, B: 0.36}
	for y := 0; y < h; y++ {
		v := (float64(y) + .5) / h
		for x := 0; x < w; x++ {
			u := (float64(x) + .5) / w
			dx := (u - .5) * s.aspect
			dy := v - .5
			r2 := func() float64 { rr := math.Hypot(dx, dy) / s.rScale; return rr * rr }()
			device := math.Exp(s.lensLog(r2) + s.panelLog(u, v))
			frame.Set(x, y, RGB{R: base.R * device, G: base.G * device, B: base.B * device})
		}
	}
	before := append([]RGB(nil), frame.Pix...)
	if err := CorrectInPlace(frame, tab); err != nil {
		t.Fatal(err)
	}

	maxLumaErr, maxRatioErr := 0.0, 0.0
	for i, got := range frame.Pix {
		want := base
		if e := math.Max(math.Abs(got.R-want.R)/want.R,
			math.Max(math.Abs(got.G-want.G)/want.G, math.Abs(got.B-want.B)/want.B)); e > maxLumaErr {
			maxLumaErr = e
		}
		old := before[i]
		if e := math.Abs((got.R/got.G)-(old.R/old.G)) / (old.R / old.G); e > maxRatioErr {
			maxRatioErr = e
		}
		if e := math.Abs((got.B/got.G)-(old.B/old.G)) / (old.B / old.G); e > maxRatioErr {
			maxRatioErr = e
		}
	}
	if maxLumaErr > 0.035 {
		t.Fatalf("correction error %.3f exceeds 3.5%%", maxLumaErr*100)
	}
	if maxRatioErr > 1e-12 {
		t.Fatalf("color ratio changed by %.2e", maxRatioErr)
	}
}

func TestTableJSONAndArbitraryDimensions(t *testing.T) {
	s := newSyntheticField()
	tab, err := Estimate(testFlatRaster(s), nil, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "correction.json")
	if err := tab.SaveJSON(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTable(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]float64{{.1, .2}, {.33, .81}, {.9, .05}} {
		a := tab.GainAt(p[0], p[1])
		b := loaded.GainAt(p[0], p[1])
		if math.Abs(a-b) > 1e-12 {
			t.Fatalf("JSON roundtrip changed gain at %v: %.8f vs %.8f", p, a, b)
		}
	}
	for _, dims := range [][2]int{{640, 360}, {33, 17}, {4096, 2160}} {
		gains, err := loaded.ResampleGain(dims[0], dims[1])
		if err != nil {
			t.Fatal(err)
		}
		if len(gains) != dims[0]*dims[1] {
			t.Fatalf("gains length = %d", len(gains))
		}
	}
}
