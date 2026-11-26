package avsync

import (
	"fmt"
	"math"
	"sort"
)

func fitLine(ps []pair, anchorMS, minDriftSpanMS float64) (lineFit, error) {
	if len(ps) < 2 {
		return lineFit{}, fmt.Errorf("need at least two pairs to fit")
	}
	xs := make([]float64, len(ps))
	es := make([]float64, len(ps))
	for i, p := range ps {
		xs[i] = p.pulseMS - anchorMS
		es[i] = p.eventMS - p.pulseMS
	}

	if len(ps) < 3 || xs[len(xs)-1]-xs[0] < minDriftSpanMS {
		return lineFit{offset: median(es), n: len(ps)}, nil
	}

	slope := theilSenSlope(xs, es)
	offset := medianResidual(es, xs, slope)
	offset, slope = huberRefine(xs, es, offset, slope)
	return lineFit{offset: offset, slope: slope, n: len(ps)}, nil
}

func theilSenSlope(xs, es []float64) float64 {
	const maxPairs = 250000
	n := len(xs)
	total := n * (n - 1) / 2
	stride := 1
	if total > maxPairs {
		stride = (total + maxPairs - 1) / maxPairs
	}

	slopes := make([]float64, 0, minInt(total, maxPairs)+1)
	k := 0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if k%stride == 0 {
				dx := xs[j] - xs[i]
				if dx > 1e-9 {
					slopes = append(slopes, (es[j]-es[i])/dx)
				}
			}
			k++
		}
	}
	if len(slopes) == 0 {
		return 0
	}
	return median(slopes)
}

func medianResidual(es, xs []float64, slope float64) float64 {
	res := make([]float64, len(es))
	for i := range es {
		res[i] = es[i] - slope*xs[i]
	}
	return median(res)
}

func huberRefine(xs, es []float64, offset, slope float64) (float64, float64) {
	for iter := 0; iter < 20; iter++ {
		res := make([]float64, len(es))
		for i := range es {
			res[i] = es[i] - offset - slope*xs[i]
		}
		scale := 1.4826 * medianAbs(res)
		if scale < 1e-9 {
			break
		}
		cutoff := 1.5 * scale

		var sw, swx, swy, swxx, swxy float64
		for i := range es {
			r := math.Abs(res[i])
			w := 1.0
			if r > cutoff {
				w = cutoff / r
			}
			sw += w
			swx += w * xs[i]
			swy += w * es[i]
			swxx += w * xs[i] * xs[i]
			swxy += w * xs[i] * es[i]
		}
		den := sw*swxx - swx*swx
		if math.Abs(den) < 1e-12 {
			break
		}
		nextSlope := (sw*swxy - swx*swy) / den
		nextOffset := (swy - nextSlope*swx) / sw
		done := math.Abs(nextOffset-offset) < 1e-9 &&
			math.Abs(nextSlope-slope) < 1e-12
		offset, slope = nextOffset, nextSlope
		if done {
			break
		}
	}
	return offset, slope
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	mid := len(v) / 2
	if len(v)%2 == 1 {
		return v[mid]
	}
	return (v[mid-1] + v[mid]) / 2
}

func medianAbs(values []float64) float64 {
	abs := make([]float64, len(values))
	for i, v := range values {
		abs[i] = math.Abs(v)
	}
	return median(abs)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
