package tearcheck

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func colorfulGlitchImage() *image.NRGBA {
	const width, height = 180, 140
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r := 128 + 80*math.Sin(float64(x)/11.0) + 35*math.Cos(float64(y)/8.0)
			g := 128 + 70*math.Sin(float64(x+y)/17.0)
			b := 128 + 75*math.Cos(float64(x-y)/13.0)
			clamp := func(v float64) uint8 {
				if v < 0 {
					return 0
				}
				if v > 255 {
					return 255
				}
				return uint8(v)
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: clamp(r),
				G: clamp(g),
				B: clamp(b),
				A: 255,
			})
		}
	}

	shiftBand := func(y0, y1, dx int) {
		for y := y0; y < y1; y++ {
			row := make([]byte, width*4)
			start := img.PixOffset(0, y)
			copy(row, img.Pix[start:start+width*4])
			for x := 0; x < width; x++ {
				srcX := x - dx
				if srcX < 0 {
					srcX += width
				}
				if srcX >= width {
					srcX -= width
				}
				si := srcX * 4
				di := start + x*4
				copy(img.Pix[di:di+4], row[si:si+4])
			}
		}
	}
	shiftBand(42, 54, 24)
	shiftBand(88, 101, -20)
	return img
}

func TestSyntheticContentGlitchPipeline(t *testing.T) {
	img := colorfulGlitchImage()
	report := AnalyzeImages([]image.Image{img}, Metadata{Codec: "h264", Width: 180, Height: 140})
	if len(report.Findings) == 0 {
		t.Fatalf("expected glitch findings: %+v", report.Summary)
	}
	for _, finding := range report.Findings {
		t.Logf("content candidate y=%d cause=%s review=%v scores=%#v chroma evidence present",
			finding.SeamY, finding.Cause, finding.ReviewRequired, finding.Scores)
	}

	actionableContent := 0
	for _, finding := range report.Findings {
		if finding.Cause == CauseContent && !finding.ReviewRequired {
			actionableContent++
		}
	}
	if actionableContent == 0 {
		t.Fatalf("expected at least one actionable content_effect, got %+v", report.Findings)
	}
	if report.Summary.Overall != CauseContent {
		t.Fatalf("overall=%q want content_effect; manual items should still be retained", report.Summary.Overall)
	}
}
