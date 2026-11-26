package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func writePNGFrames(outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for frame := 0; frame < 12; frame++ {
		img := image.NewRGBA(image.Rect(0, 0, 96, 64))
		drawRect(img, image.Rect(10+frame*2, 12, 39+frame*2, 52), color.RGBA{R: 0x2f, G: 0x80, B: 0xed, A: 255})
		drawRect(img, image.Rect(52, 12, 76, 52), color.RGBA{R: 0xf2, G: 0x99, B: 0x4a, A: 140})
		drawCircle(img, 20+frame*2, 28, 8, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		path := filepath.Join(outDir, frameName(frame))
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func frameName(i int) string {
	const digits = "0123456789"
	b := []byte("000.png")
	b[0] = digits[i/100]
	b[1] = digits[i/10%10]
	b[2] = digits[i%10]
	return string(b)
}

func drawRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	b := img.Bounds()
	r = r.Intersect(b)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			blendPixel(img, x, y, c)
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= radius*radius {
				blendPixel(img, x, y, c)
			}
		}
	}
}

func blendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	if !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	i := img.PixOffset(x, y)
	a := float64(c.A) / 255
	img.Pix[i] = byte(float64(c.R)*a + float64(img.Pix[i])*(1-a))
	img.Pix[i+1] = byte(float64(c.G)*a + float64(img.Pix[i+1])*(1-a))
	img.Pix[i+2] = byte(float64(c.B)*a + float64(img.Pix[i+2])*(1-a))
	img.Pix[i+3] = byte(float64(c.A) + float64(img.Pix[i+3])*(1-a))
}
