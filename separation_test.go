package flatfield

import (
	"math"
	"strings"
	"testing"
)

func TestMultiplicativeCorrectionPreservesRealSceneGradient(t *testing.T) {
	s := newSyntheticField()
	flat, err := Estimate(testFlatRaster(s), nil, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	noGradient, err := flat.ForKind(KindDeviceNoGradient, -1)
	if err != nil {
		t.Fatal(err)
	}

	const w, h = 91, 69
	observed, err := NewFrame(w, h)
	if err != nil {
		t.Fatal(err)
	}
	original, err := NewFrame(w, h)
	if err != nil {
		t.Fatal(err)
	}
	base := RGB{R: 0.58, G: 0.52, B: 0.46}
	for y := 0; y < h; y++ {
		v := (float64(y) + .5) / h
		for x := 0; x < w; x++ {
			u := (float64(x) + .5) / w
			dx := (u - .5) * s.aspect
			dy := v - .5
			r := math.Hypot(dx, dy) / s.rScale
			device := math.Exp(s.lensLog(r*r) + s.panelLog(u, v))
			sceneGradient := math.Exp(0.20*dx + 0.11*dy)
			scene := RGB{
				R: base.R * sceneGradient,
				G: base.G * sceneGradient,
				B: base.B * sceneGradient,
			}
			i := y*w + x
			original.Pix[i] = scene
			observed.Pix[i] = ApplyGain(scene, device)
		}
	}
	if err := CorrectInPlace(observed, noGradient); err != nil {
		t.Fatal(err)
	}

	maxErr := 0.0
	for i, got := range observed.Pix {
		want := original.Pix[i]
		err := math.Max(math.Abs(got.R-want.R)/want.R,
			math.Max(math.Abs(got.G-want.G)/want.G, math.Abs(got.B-want.B)/want.B))
		maxErr = math.Max(maxErr, err)
	}
	if maxErr > 0.04 {
		t.Fatalf("scene/gradient restoration error %.2f%%", maxErr*100)
	}

	grad, err := EstimateSceneGradient(&FrameRasterAdapter{Frame: original}, 16, 12)
	if err != nil {
		t.Fatal(err)
	}
	correctedGrad, err := EstimateSceneGradient(&FrameRasterAdapter{Frame: observed}, 16, 12)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(grad.MagnitudePerHeight-correctedGrad.MagnitudePerHeight) > 0.025 {
		t.Fatalf("gradient changed from %.4f to %.4f", grad.MagnitudePerHeight, correctedGrad.MagnitudePerHeight)
	}
}

type FrameRasterAdapter struct{ Frame *Frame }

func (f *FrameRasterAdapter) Size() (int, int) { return f.Frame.Width, f.Frame.Height }
func (f *FrameRasterAdapter) LinearRGBAt(x, y int) RGB {
	return f.Frame.LinearRGBAt(x, y)
}

func TestIndependentLensTableAttribution(t *testing.T) {
	s := newSyntheticField()
	lensOnly := FuncRaster{W: testW, H: testH, Fn: func(x, y int) RGB {
		_, _, _, _, _, r2 := s.coords(x, y)
		v := math.Exp(s.lensLog(r2))
		return RGB{R: 0.8 * v, G: 0.8 * v, B: 0.8 * v}
	}}
	lensEstimate, err := Estimate(lensOnly, nil, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	lensTable, err := lensEstimate.ForKind(KindLens, -1)
	if err != nil {
		t.Fatal(err)
	}

	o := testOptions()
	o.Lens = lensTable
	tab, err := Estimate(testFlatRaster(s), nil, o)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Report.Lens.Reference == "" || !strings.Contains(tab.Report.Identifiability, "lens_independently_referenced") {
		t.Fatalf("expected independent lens attribution, got ref=%q id=%q",
			tab.Report.Lens.Reference, tab.Report.Identifiability)
	}
	if tab.Report.Confidence < 0.85 {
		t.Fatalf("independent confidence = %.2f", tab.Report.Confidence)
	}

	panel, err := tab.ForKind(KindPanel, -1)
	if err != nil {
		t.Fatal(err)
	}
	cornerVariation := 0.0
	for _, c := range panel.Report.Panel.CornerResponse {
		cornerVariation = math.Max(cornerVariation, math.Abs(math.Log(c.Response)))
	}
	if cornerVariation < 0.10 {
		t.Fatalf("panel corner variation too small: %.4f", cornerVariation)
	}
}

func TestFullDeviceTableCorrectsAffinePanelField(t *testing.T) {
	s := newSyntheticField()
	tab, err := Estimate(testFlatRaster(s), nil, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	const w, h = 75, 55
	frame, err := NewFrame(w, h)
	if err != nil {
		t.Fatal(err)
	}
	base := RGB{R: 0.6, G: 0.5, B: 0.4}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			frame.Pix[i] = ApplyGain(base, math.Exp(s.totalLog(x*testW/w, y*testH/h)))
		}
	}
	if err := CorrectInPlace(frame, tab); err != nil {
		t.Fatal(err)
	}
	maxErr := 0.0
	for _, got := range frame.Pix {
		maxErr = math.Max(maxErr, math.Max(math.Abs(got.R-base.R)/base.R,
			math.Max(math.Abs(got.G-base.G)/base.G, math.Abs(got.B-base.B)/base.B)))
	}
	if maxErr > 0.04 {
		t.Fatalf("full device correction error %.2f%%", maxErr*100)
	}
}
