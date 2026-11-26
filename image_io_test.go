package flatfield

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestCorrectImagePreservesLinearChannelRatios(t *testing.T) {
	value := color.NRGBA{R: 188, G: 116, B: 72, A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, value)
		}
	}

	two := make([]float64, 4)
	for i := range two {
		two[i] = 2
	}
	tab := &Table{
		GridWidth: 2, GridHeight: 2,
		OpticalCenterX: .5, OpticalCenterY: .5,
		Response: append([]float64(nil), two...),
		Gain:     []float64{2, 2, 2, 2},
	}
	for i := range tab.Response {
		tab.Response[i] = .5
	}

	out, err := CorrectImage(img, tab)
	if err != nil {
		t.Fatal(err)
	}
	inLin := ImageRaster{Image: img}.LinearRGBAt(0, 0)
	out64 := out.NRGBA64At(0, 0)
	outLin := RGB{
		R: srgbToLinear(float64(out64.R) / 65535),
		G: srgbToLinear(float64(out64.G) / 65535),
		B: srgbToLinear(float64(out64.B) / 65535),
	}

	wantRG := inLin.R / inLin.G
	wantBG := inLin.B / inLin.G
	gotRG := outLin.R / outLin.G
	gotBG := outLin.B / outLin.G
	if math.Abs(gotRG-wantRG)/wantRG > 0.005 {
		t.Fatalf("R/G ratio changed: got %.5f want %.5f", gotRG, wantRG)
	}
	if math.Abs(gotBG-wantBG)/wantBG > 0.005 {
		t.Fatalf("B/G ratio changed: got %.5f want %.5f", gotBG, wantBG)
	}
}
