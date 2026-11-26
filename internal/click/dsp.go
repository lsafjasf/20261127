package click

import (
	"math"
	"sort"
)

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	cp := append([]float64(nil), v...)
	sort.Float64s(cp)
	return percentileSorted(cp, 0.5)
}

func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	cp := append([]float64(nil), v...)
	sort.Float64s(cp)
	return percentileSorted(cp, p)
}

func percentileSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo]*(float64(hi)-pos) + sorted[hi]*(pos-float64(lo))
}

func madScale(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	m := median(v)
	dev := make([]float64, len(v))
	for i, x := range v {
		dev[i] = math.Abs(x - m)
	}
	return 1.4826 * median(dev)
}

func rms(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum / float64(len(v)))
}

func wrapPhase(v float64) float64 {
	for v > math.Pi {
		v -= 2 * math.Pi
	}
	for v < -math.Pi {
		v += 2 * math.Pi
	}
	return v
}

type biquad struct {
	b0, b1, b2, a1, a2 float64
	z1, z2             float64
}

func (b *biquad) reset() {
	b.z1, b.z2 = 0, 0
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.z1
	b.z1 = b.b1*x - b.a1*y + b.z2
	b.z2 = b.b2*x - b.a2*y
	return y
}

func newLowpass(fs, cutoff float64) *biquad {
	if cutoff >= fs*0.49 {
		cutoff = fs * 0.49
	}
	w := 2 * math.Pi * cutoff / fs
	alpha := math.Sin(w) / (2 * math.Sqrt2)
	cosw := math.Cos(w)
	a0 := 1 + alpha
	return &biquad{
		b0: (1 - cosw) / 2 / a0,
		b1: (1 - cosw) / a0,
		b2: (1 - cosw) / 2 / a0,
		a1: -2 * cosw / a0,
		a2: (1 - alpha) / a0,
	}
}

func newBandpass(fs, lo, hi float64) *biquad {
	lo = math.Max(1, lo)
	hi = math.Min(fs*0.48, hi)
	if hi <= lo {
		hi = math.Min(fs*0.48, lo*1.2)
	}
	center := math.Sqrt(lo * hi)
	q := center / (hi - lo)
	if q < 0.15 {
		q = 0.15
	}
	w := 2 * math.Pi * center / fs
	alpha := math.Sin(w) / (2 * q)
	cosw := math.Cos(w)
	a0 := 1 + alpha
	return &biquad{
		b0: alpha / a0,
		b1: 0,
		b2: -alpha / a0,
		a1: -2 * cosw / a0,
		a2: (1 - alpha) / a0,
	}
}

func filter(input []float64, b *biquad) []float64 {
	out := make([]float64, len(input))
	b.reset()
	for i, x := range input {
		out[i] = b.process(x)
	}
	return out
}

func absDiff(input []float64) []float64 {
	out := make([]float64, len(input))
	for i := 1; i < len(input); i++ {
		out[i] = math.Abs(input[i] - input[i-1])
	}
	return out
}

func secondDiff(input []float64) []float64 {
	out := make([]float64, len(input))
	for i := 2; i < len(input); i++ {
		out[i] = math.Abs(input[i] - 2*input[i-1] + input[i-2])
	}
	return out
}

func stepEdgeDiff(input []float64, sampleRate int) []float64 {
	win := int(math.Round(float64(sampleRate) * 0.002))
	if win < 8 {
		win = 8
	}
	out := make([]float64, len(input))
	if len(input) < 2*win+2 {
		return out
	}
	var before, after float64
	for i := 0; i < win; i++ {
		before += input[i]
	}
	for i := win; i < 2*win; i++ {
		after += input[i]
	}
	prev := after/float64(win) - before/float64(win)
	for n := win + 1; n+win < len(input); n++ {
		before += input[n-1] - input[n-1-win]
		after += input[n-1+win] - input[n-1]
		current := after/float64(win) - before/float64(win)
		out[n] = math.Abs(current - prev)
		prev = current
	}
	return out
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func clampIndex(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
