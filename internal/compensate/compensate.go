// Package compensate applies spatially local black-level subtraction that is
// bounded by the low-light noise floor and by an absolute cap, so genuine
// night scenes keep their shadow detail instead of being crushed.
package compensate

import (
	"math"

	"blacklift/internal/estim"
	"blacklift/internal/frameio"
)

// Options bounds every compensation decision.
type Options struct {
	// MaxComp is the largest subtraction allowed anywhere (0..1 luma).
	MaxComp float64
	// NoiseK: local subtraction never exceeds max(0, b - k*sigma).
	NoiseK float64
	// StyleScale multiplies compensation on intentional-lift shots.
	StyleScale float64
	// BlackGuard: pixels already below this luma are never driven to zero.
	BlackGuard float64
}

// DefaultOptions returns conservative bounds (~16-level cap on 8-bit).
func DefaultOptions() Options {
	return Options{
		MaxComp:    16.0 / 255.0,
		NoiseK:     1.5,
		StyleScale: 0.0,
		BlackGuard: 2.0 / 255.0,
	}
}

// FrameResult reports what happened to one frame.
type FrameResult struct {
	Index              int
	Path               string
	Width, Height      int
	StyleLift          bool
	StyleScore         float64
	Sigma              float64
	MaskBase           float64
	MaskCorner         float64
	MaskSpread         float64
	MaskRSquared       float64
	RawMean            float64
	RawMax             float64
	BoundedMean        float64
	BoundedMax         float64
	AppliedMean        float64
	AppliedMax         float64
	GuardFrac          float64
	ShadowDetailBefore float64
	ShadowDetailAfter  float64
	NewClippedLuma     int64
	NewClippedFrac     float64
	NewClippedAnyChan  int64
	NewClippedAnyFrac  float64
}

// BuildCompMap turns a fitted mask into a per-pixel raw compensation map.
func BuildCompMap(m *estim.Mask, w, h int) []float64 {
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		ny := (float64(y)+0.5)/float64(h)*2 - 1
		for x := 0; x < w; x++ {
			nx := (float64(x)+0.5)/float64(w)*2 - 1
			v := m.At(nx, ny)
			if v < 0 {
				v = 0
			}
			out[y*w+x] = v
		}
	}
	return out
}

// Apply subtracts a noise-floor-bounded, capped local map from the frame and
// measures clipping and shadow detail. It mutates the frame in place.
func Apply(fr *frameio.Frame, raw []float64, sigma float64, styleLift bool, o Options) *FrameResult {
	n := fr.Width * fr.Height
	res := &FrameResult{Index: -1, Path: fr.Path, Width: fr.Width, Height: fr.Height, Sigma: sigma}
	bounded := make([]float64, n)
	applied := make([]float64, n)
	scale := 1.0
	if styleLift {
		scale = o.StyleScale
	}
	dark := 0

	for i := 0; i < n; i++ {
		b := raw[i]
		res.RawMean += b
		if b > res.RawMax {
			res.RawMax = b
		}
		// Noise floor: never subtract into the local noise population.
		bf := b - o.NoiseK*sigma
		if bf < 0 {
			bf = 0
		}
		if bf > o.MaxComp {
			bf = o.MaxComp
		}
		bf *= scale
		bounded[i] = bf
		res.BoundedMean += bf
		if bf > res.BoundedMax {
			res.BoundedMax = bf
		}

		yBefore := fr.Y[i]
		c := bf
		if yBefore-c < o.BlackGuard {
			c = math.Max(0, yBefore-o.BlackGuard)
			if c < bf {
				res.GuardFrac++
			}
		}
		applied[i] = c
		res.AppliedMean += c
		if c > res.AppliedMax {
			res.AppliedMax = c
		}

		if wasDark(yBefore) {
			dark++
			res.ShadowDetailBefore += yBefore
			res.ShadowDetailAfter += math.Max(o.BlackGuard, yBefore-c)
		}
		if yBefore > 0 && yBefore-c <= 0 {
			res.NewClippedLuma++
		}
	}

	for i := 0; i < n; i++ {
		c := applied[i]
		if c <= 0 {
			continue
		}
		if (fr.R[i] > 0 && fr.R[i]-c <= 0) || (fr.G[i] > 0 && fr.G[i]-c <= 0) || (fr.B[i] > 0 && fr.B[i]-c <= 0) {
			res.NewClippedAnyChan++
		}
	}

	applyToChannels(fr, applied)
	res.GuardFrac /= float64(n)
	res.RawMean /= float64(n)
	res.BoundedMean /= float64(n)
	res.AppliedMean /= float64(n)
	if dark > 0 {
		res.ShadowDetailBefore /= float64(dark)
		res.ShadowDetailAfter /= float64(dark)
	}
	res.NewClippedFrac = float64(res.NewClippedLuma) / float64(n)
	res.NewClippedAnyFrac = float64(res.NewClippedAnyChan) / float64(n)
	return res
}

func applyToChannels(fr *frameio.Frame, c []float64) {
	for i := range c {
		fr.R[i] = clamp01(fr.R[i] - c[i])
		fr.G[i] = clamp01(fr.G[i] - c[i])
		fr.B[i] = clamp01(fr.B[i] - c[i])
		fr.Y[i] = clamp01(fr.Y[i] - c[i])
	}
}

func wasDark(y float64) bool { return y < 0.10 }

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
