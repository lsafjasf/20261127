// Package flatfield estimates reusable two-dimensional flat-field correction
// tables using only the Go standard library.
//
// The coordinate convention is normalized pixel centers:
//
//	u = (x + 0.5) / width
//	v = (y + 0.5) / height
//
// Thus a table estimated from one resolution can be sampled at any other
// resolution with bilinear interpolation.
package flatfield

import (
	"errors"
	"math"
)

// RGB is one linear-light, straight-alpha-independent color sample.
type RGB struct {
	R float64
	G float64
	B float64
}

// Raster is the minimal image interface required by estimation.
type Raster interface {
	Size() (width, height int)
	LinearRGBAt(x, y int) RGB
}

// Frame is an in-memory linear RGB raster.
type Frame struct {
	Width  int
	Height int
	Pix    []RGB
}

// NewFrame validates and allocates a frame.
func NewFrame(width, height int) (*Frame, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("flatfield: frame dimensions must be positive")
	}
	if width > math.MaxInt/height {
		return nil, errors.New("flatfield: frame dimensions overflow")
	}
	return &Frame{Width: width, Height: height, Pix: make([]RGB, width*height)}, nil
}

// Size implements Raster.
func (f *Frame) Size() (int, int) { return f.Width, f.Height }

// LinearRGBAt implements Raster.
func (f *Frame) LinearRGBAt(x, y int) RGB {
	return f.Pix[y*f.Width+x]
}

// Set sets a pixel.
func (f *Frame) Set(x, y int, c RGB) {
	f.Pix[y*f.Width+x] = c
}

// FuncRaster adapts a generator function to Raster.
type FuncRaster struct {
	W, H int
	Fn   func(x, y int) RGB
}

// Size implements Raster.
func (r FuncRaster) Size() (int, int) { return r.W, r.H }

// LinearRGBAt implements Raster.
func (r FuncRaster) LinearRGBAt(x, y int) RGB { return r.Fn(x, y) }

func dims(r Raster) (int, int, error) {
	if r == nil {
		return 0, 0, errors.New("flatfield: nil raster")
	}
	w, h := r.Size()
	if w <= 0 || h <= 0 {
		return 0, 0, errors.New("flatfield: raster dimensions must be positive")
	}
	return w, h, nil
}

func luma(c RGB) float64 {
	return (c.R + c.G + c.B) / 3.0
}

func clampFloat(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	return math.Min(hi, math.Max(lo, v))
}
