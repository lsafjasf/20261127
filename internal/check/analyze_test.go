package check

import (
	"image"
	"testing"

	"proalpha/internal/imagex"
)

func testFrame(r, g, b, a uint16) *image.NRGBA64 {
	frame := image.NewNRGBA64(image.Rect(0, 0, 4, 4))
	for i := range frame.Pix {
		frame.Pix[i] = 0
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			o := frame.PixOffset(x, y)
			frame.Pix[o+0] = byte(r >> 8)
			frame.Pix[o+1] = byte(r)
			frame.Pix[o+2] = byte(g >> 8)
			frame.Pix[o+3] = byte(g)
			frame.Pix[o+4] = byte(b >> 8)
			frame.Pix[o+5] = byte(b)
			frame.Pix[o+6] = byte(a >> 8)
			frame.Pix[o+7] = byte(a)
		}
	}
	return frame
}

func TestAnalyzeDetectsPremultipliedReference(t *testing.T) {
	source := testFrame(imagex.Max16, 20000, 10000, 32768)
	variant := imagex.Premultiply(source)
	result := AnalyzeFrame(FrameInput{Index: 0, Source: source, Variant: variant})

	if !result.Report.Unpremultiply {
		t.Fatalf("expected unpremultiply, models=%+v", result.Report.Models)
	}
	o := result.Repaired.PixOffset(0, 0)
	r := uint16(result.Repaired.Pix[o])<<8 | uint16(result.Repaired.Pix[o+1])
	if r < 64500 {
		t.Fatalf("repaired red = %d, want near 65535", r)
	}
}

func TestAnalyzeAttachesDroppedAlpha(t *testing.T) {
	source := testFrame(imagex.Max16, 20000, 10000, 32768)
	variant := testFrame(imagex.Max16, 20000, 10000, imagex.Max16)
	result := AnalyzeFrame(FrameInput{Index: 2, Source: source, Variant: variant})

	if !result.Report.AttachSourceAlpha {
		t.Fatalf("expected source alpha attachment")
	}
	o := result.Repaired.PixOffset(0, 0)
	a := uint16(result.Repaired.Pix[o+6])<<8 | uint16(result.Repaired.Pix[o+7])
	if a != 32768 {
		t.Fatalf("repaired alpha = %d", a)
	}
}
