package video

import (
	"fmt"
	"math"

	"colorfix/internal/color"
)

type Range string

const (
	RangeLimited Range = "tv"
	RangeFull    Range = "pc"
)

type Frame struct {
	Width  int
	Height int
	Y      []byte
	U      []byte
	V      []byte
	Range  Range
	Time   float64
}

func NewFrame(width, height int, raw []byte, rng Range) (*Frame, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid frame size %dx%d", width, height)
	}
	want := width * height * 3
	if len(raw) < want {
		return nil, fmt.Errorf("short yuv444p frame: got %d, want %d", len(raw), want)
	}
	n := width * height
	return &Frame{
		Width: width, Height: height,
		Y: raw[:n], U: raw[n : 2*n], V: raw[2*n : 3*n],
		Range: rng,
	}, nil
}

func ParseRange(name string) Range {
	switch name {
	case "pc", "full", "jpeg":
		return RangeFull
	default:
		return RangeLimited
	}
}

// YUV returns normalized Y, U and V. For limited range, 16..235 and 16..240
// are mapped to 0..1. Negative and above-one values are preserved.
func (f *Frame) YUV(x, y int) (yn, un, vn float64) {
	i := y*f.Width + x
	if f.Range == RangeFull {
		yn = float64(f.Y[i]) / 255.0
		un = float64(f.U[i]-128) / 255.0
		vn = float64(f.V[i]-128) / 255.0
		return
	}
	yn = (float64(f.Y[i]) - 16.0) / 219.0
	un = (float64(f.U[i]) - 128.0) / 112.0
	vn = (float64(f.V[i]) - 128.0) / 112.0
	return
}

func (f *Frame) RGB(matrix color.Matrix, x, y int) color.RGB {
	yn, un, vn := f.YUV(x, y)
	r, g, b := color.Decode(matrix, yn, un, vn)
	return color.RGB{R: r, G: g, B: b}
}

func (f *Frame) MeanChroma() float64 {
	step := maxInt(1, minInt(f.Width, f.Height)/96)
	var sum float64
	var n int
	for y := step / 2; y < f.Height; y += step {
		for x := step / 2; x < f.Width; x += step {
			_, u, v := f.YUV(x, y)
			sum += u*u + v*v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(n))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
