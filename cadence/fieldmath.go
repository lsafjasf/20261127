package cadence

import (
	"math"
	"sort"
)

const (
	descriptorCols = 12
	descriptorRows = 8
)

type descriptor struct {
	values []float64
	mean   float64
}

func makeDescriptor(im Image) descriptor {
	out := make([]float64, descriptorCols*descriptorRows)
	var total float64
	for gy := 0; gy < descriptorRows; gy++ {
		y0 := gy * im.Height / descriptorRows
		y1 := (gy + 1) * im.Height / descriptorRows
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for gx := 0; gx < descriptorCols; gx++ {
			x0 := gx * im.Width / descriptorCols
			x1 := (gx + 1) * im.Width / descriptorCols
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sum, count int
			for y := y0; y < y1 && y < im.Height; y++ {
				row := y * im.Width
				for x := x0; x < x1 && x < im.Width; x++ {
					sum += int(im.Pix[row+x])
					count++
				}
			}
			if count > 0 {
				out[gy*descriptorCols+gx] = float64(sum) / float64(count)
				total += out[gy*descriptorCols+gx]
			}
		}
	}
	mean := total / float64(len(out))
	for i := range out {
		out[i] -= mean
	}
	return descriptor{values: out, mean: mean}
}

// fieldDistance combines mean-subtracted structure and gross level. Ignoring
// the DC component makes duplicate fields survive codec brightness wobble;
// retaining a small DC term prevents unrelated flat frames looking identical.
func fieldDistance(a, b descriptor) float64 {
	if len(a.values) != len(b.values) || len(a.values) == 0 {
		return 1
	}
	var structural float64
	for i := range a.values {
		structural += math.Abs(a.values[i] - b.values[i])
	}
	structural /= float64(len(a.values)) * 255
	level := math.Abs(a.mean-b.mean) / 255
	return structural + 0.15*level
}

func median(values []float64) float64 {
	return percentile(values, 0.5)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	pos := p * float64(len(cp)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return cp[lo]
	}
	frac := pos - float64(lo)
	return cp[lo]*(1-frac) + cp[hi]*frac
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func ratioScore(num, den, floor float64) float64 {
	if den <= 1e-9 {
		return 0
	}
	v := (num/den - 1) / floor
	return clamp(v, 0, 1)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func stddev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	m := mean(values)
	var sum float64
	for _, v := range values {
		d := v - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(values)-1))
}
