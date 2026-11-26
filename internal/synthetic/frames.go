package synthetic

import (
	"image"
	"math"
	"path/filepath"

	"proalpha/internal/imagex"
)

func Frame(width, height, index, frameCount int) *image.NRGBA64 {
	frame := image.NewNRGBA64(image.Rect(0, 0, width, height))
	cy := float64(height) * 0.48
	cx := float64(width) * (0.38 + 0.04*math.Sin(float64(index)*math.Pi/2))
	radius := float64(minInt(width, height)) * 0.34

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			fx := float64(x)
			fy := float64(y)
			dx := (fx - cx) / radius
			dy := (fy - cy) / radius
			dist := math.Sqrt(dx*dx + dy*dy)

			var r, g, b, a float64
			switch {
			case dist <= 1:
				edge := math.Max(0, 1-dist)
				a = 0.25 + 0.75*edge
				r, g, b = 1.0, 0.12+0.2*edge, 0.08
			case x > width*55/100 && y > height*18/100 && y < height*82/100:
				a = 0.42
				r, g, b = 0.1, 0.95, 0.25
			case (x+y+index*7)%17 < 8 && y > height*68/100:
				a = 1
				r, g, b = 0.15, 0.35, 1.0
			default:
				a = 0
				r, g, b = 0.05, 0.05, 0.05
			}

			if frameCount > 1 {
				a *= 0.82 + 0.18*float64(index+1)/float64(frameCount)
			}
			o := frame.PixOffset(x, y)
			put16(frame.Pix[o+0:o+2], scale(r))
			put16(frame.Pix[o+2:o+4], scale(g))
			put16(frame.Pix[o+4:o+6], scale(b))
			put16(frame.Pix[o+6:o+8], scale(a))
		}
	}
	return frame
}

func WriteSequence(dir string, count, width, height int) ([]string, error) {
	paths := make([]string, 0, count)
	for i := 0; i < count; i++ {
		path := filepath.Join(dir, frameName(i))
		frame := Frame(width, height, i, count)
		if err := imagex.SavePNG(path, frame); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func frameName(index int) string {
	return "frame_" + pad4(index) + ".png"
}

func pad4(index int) string {
	s := ""
	for n := index; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

func scale(value float64) uint16 {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	return uint16(math.Round(value * imagex.Max16))
}

func put16(dst []byte, value uint16) {
	dst[0] = byte(value >> 8)
	dst[1] = byte(value)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
