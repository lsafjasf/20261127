package imagex

import (
	"image"
	"testing"
)

func setPixel(f *image.NRGBA64, x, y int, r, g, b, a uint16) {
	o := f.PixOffset(x, y)
	putChannel(f.Pix[o+0:o+2], r)
	putChannel(f.Pix[o+2:o+4], g)
	putChannel(f.Pix[o+4:o+6], b)
	putChannel(f.Pix[o+6:o+8], a)
}

func TestPremultiplyAndUnpremultiplyRoundTrip(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	setPixel(src, 0, 0, Max16, 20000, 10000, 32768)

	premultiplied := Premultiply(src)
	r, _, _, a := channelsAt(premultiplied, 0)
	if a != 32768 {
		t.Fatalf("alpha changed: %d", a)
	}
	if r < 32000 || r > 33500 {
		t.Fatalf("premultiplied red = %d, want about 32768", r)
	}

	restored := Unpremultiply(premultiplied)
	r, g, b, a := channelsAt(restored, 0)
	if a != 32768 || absInt(r, Max16) > 80 || absInt(g, 20000) > 80 || absInt(b, 10000) > 80 {
		t.Fatalf("restored = (%d,%d,%d,%d)", r, g, b, a)
	}
}

func TestClassifyStraightAndPremultipliedFrames(t *testing.T) {
	straight := image.NewNRGBA64(image.Rect(0, 0, 4, 4))
	premultiplied := image.NewNRGBA64(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			setPixel(straight, x, y, Max16, 12000, 8000, 32768)
			setPixel(premultiplied, x, y, 32768, 6000, 4000, 32768)
		}
	}

	model, _ := ClassifySingleAlpha(straight)
	if model != AlphaModelStraightConfirmed {
		t.Fatalf("straight model = %q", model)
	}
	model, _ = ClassifySingleAlpha(premultiplied)
	if model != AlphaModelLikelyPremultiplied {
		t.Fatalf("premultiplied model = %q", model)
	}
}

func absInt(a, b uint16) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
