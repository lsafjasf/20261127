package preview

import (
	"image/color"
	"testing"
)

func TestCompositeStraightAlpha(t *testing.T) {
	w, h := 2, 1
	rgba := []byte{
		255, 0, 0, 255,
		255, 0, 0, 255,
	}
	alpha := []byte{0, 128}

	black, white, stats := Composite(w, h, rgba, alpha)

	if got := black.RGBAAt(0, 0); got != (color.RGBA{R: 0, G: 0, B: 0, A: 255}) {
		t.Fatalf("transparent over black = %v", got)
	}
	if got := white.RGBAAt(0, 0); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("transparent over white = %v", got)
	}
	semiBlack := black.RGBAAt(1, 0)
	semiWhite := white.RGBAAt(1, 0)
	if !near(semiBlack.R, 128, 1) || semiBlack.G != 0 || semiBlack.B != 0 {
		t.Fatalf("semi-red over black = %v", semiBlack)
	}
	if !near(semiWhite.R, 255, 1) || !near(semiWhite.G, 127, 2) || !near(semiWhite.B, 127, 2) {
		t.Fatalf("semi-red over white = %v", semiWhite)
	}
	if stats.TransparentPixels != 1 || stats.PartialPixels != 1 || stats.AlphaMin != 0 || stats.AlphaMax != 128 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.BlackWhiteMeanDiff < 80 {
		t.Fatalf("black/white difference too small: %.2f", stats.BlackWhiteMeanDiff)
	}
}

func TestDoublePremultiplicationAssessment(t *testing.T) {
	w, h := 3, 3
	rgba := make([]byte, w*h*4)
	alpha := make([]byte, w*h)
	for p := 0; p < w*h; p++ {
		rgba[p*4] = 255
		rgba[p*4+3] = 255
	}
	alpha[4] = 128

	_, _, stats := CompositeDoublePremultiplication(w, h, rgba, alpha)
	if stats.WhiteHaloLumaDistance >= -12 {
		t.Fatalf("expected dark halo, got %.2f", stats.WhiteHaloLumaDistance)
	}
	assessment := Assess(stats, false)
	if assessment.Class != "composition_error" {
		t.Fatalf("assessment=%+v", assessment)
	}
}

func near(got, want uint8, tolerance int) bool {
	d := int(got) - int(want)
	if d < 0 {
		d = -d
	}
	return d <= tolerance
}
