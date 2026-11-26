package imagex

import (
	"image"
	"image/color"
)

func CompositeChecker(src *image.NRGBA64, cell int, light, dark uint16) *image.NRGBA64 {
	if cell <= 0 {
		cell = 16
	}
	dst := image.NewNRGBA64(src.Rect)
	bounds := src.Bounds()
	width := bounds.Dx()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			checkerLight := ((x-bounds.Min.X)/cell+(y-bounds.Min.Y)/cell)%2 == 0
			bg := dark
			if checkerLight {
				bg = light
			}
			i := (y-bounds.Min.Y)*width + (x - bounds.Min.X)
			r, g, b, a := channelsAt(src, i)
			af := float64(a) / Max16
			outR := clamp16(float64(r)*af + float64(bg)*(1-af))
			outG := clamp16(float64(g)*af + float64(bg)*(1-af))
			outB := clamp16(float64(b)*af + float64(bg)*(1-af))
			o := dst.PixOffset(x, y)
			putChannel(dst.Pix[o+0:o+2], outR)
			putChannel(dst.Pix[o+2:o+4], outG)
			putChannel(dst.Pix[o+4:o+6], outB)
			putChannel(dst.Pix[o+6:o+8], Max16)
		}
	}
	return dst
}

func CompositeColor(src *image.NRGBA64, background color.RGBA64) *image.NRGBA64 {
	dst := image.NewNRGBA64(src.Rect)
	bounds := src.Bounds()
	width := bounds.Dx()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			i := (y-bounds.Min.Y)*width + (x - bounds.Min.X)
			r, g, b, a := channelsAt(src, i)
			af := float64(a) / Max16
			outR := clamp16(float64(r)*af + float64(background.R)*(1-af))
			outG := clamp16(float64(g)*af + float64(background.G)*(1-af))
			outB := clamp16(float64(b)*af + float64(background.B)*(1-af))
			o := dst.PixOffset(x, y)
			putChannel(dst.Pix[o+0:o+2], outR)
			putChannel(dst.Pix[o+2:o+4], outG)
			putChannel(dst.Pix[o+4:o+6], outB)
			putChannel(dst.Pix[o+6:o+8], Max16)
		}
	}
	return dst
}

func HorizontalPair(left, right image.Image) *image.RGBA {
	lb := left.Bounds()
	rb := right.Bounds()
	height := maxInt(lb.Dy(), rb.Dy())
	width := lb.Dx() + rb.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	drawAt(dst, left, 0)
	drawAt(dst, right, lb.Dx())
	return dst
}

func drawAt(dst *image.RGBA, src image.Image, xOffset int) {
	bounds := src.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(x-bounds.Min.X+xOffset, y-bounds.Min.Y, src.At(x, y))
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
