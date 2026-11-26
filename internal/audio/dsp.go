package audio

import (
	"math"
	"sort"
)

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
}

func lowPassSection(sampleRate int, cutoff, q float64) biquad {
	w0 := 2 * math.Pi * cutoff / float64(sampleRate)
	alpha := math.Sin(w0) / (2 * q)
	cosW := math.Cos(w0)
	a0 := 1 + alpha
	return biquad{
		b0: (1 - cosW) / 2 / a0,
		b1: (1 - cosW) / a0,
		b2: (1 - cosW) / 2 / a0,
		a1: -2 * cosW / a0,
		a2: (1 - alpha) / a0,
	}
}

func highPassSection(sampleRate int, cutoff, q float64) biquad {
	w0 := 2 * math.Pi * cutoff / float64(sampleRate)
	alpha := math.Sin(w0) / (2 * q)
	cosW := math.Cos(w0)
	a0 := 1 + alpha
	return biquad{
		b0: (1 + cosW) / 2 / a0,
		b1: -(1 + cosW) / a0,
		b2: (1 + cosW) / 2 / a0,
		a1: -2 * cosW / a0,
		a2: (1 - alpha) / a0,
	}
}

func butterworthSections(sampleRate int, cutoff float64, order int, highPass bool) []biquad {
	if order < 2 {
		order = 2
	}
	if order%2 != 0 {
		order++
	}
	sections := make([]biquad, 0, order/2)
	for k := 1; k <= order/2; k++ {
		theta := math.Pi * float64(2*k-1) / float64(2*order)
		q := 1 / (2 * math.Cos(theta))
		if highPass {
			sections = append(sections, highPassSection(sampleRate, cutoff, q))
		} else {
			sections = append(sections, lowPassSection(sampleRate, cutoff, q))
		}
	}
	return sections
}

func filterForward(x []float64, sections []biquad) []float64 {
	out := append([]float64(nil), x...)
	for _, s := range sections {
		var z1, z2 float64
		for i, in := range out {
			y := s.b0*in + z1
			z1 = s.b1*in - s.a1*y + z2
			z2 = s.b2*in - s.a2*y
			out[i] = y
		}
	}
	return out
}

func filterZeroPhase(x []float64, sections []biquad) []float64 {
	if len(x) < 2 {
		return append([]float64(nil), x...)
	}
	padded := reflectPad(x, min(len(x)-1, 4096))
	y := filterForward(padded, sections)
	reverse(y)
	y = filterForward(y, sections)
	reverse(y)
	pad := (len(y) - len(x)) / 2
	return y[pad : pad+len(x)]
}

func reflectPad(x []float64, pad int) []float64 {
	if pad <= 0 {
		return append([]float64(nil), x...)
	}
	out := make([]float64, len(x)+2*pad)
	copy(out[pad:], x)
	for i := 0; i < pad; i++ {
		out[pad-1-i] = x[min(i+1, len(x)-1)]
		out[pad+len(x)+i] = x[max(len(x)-2-i, 0)]
	}
	return out
}

func reverse(x []float64) {
	for i, j := 0, len(x)-1; i < j; i, j = i+1, j-1 {
		x[i], x[j] = x[j], x[i]
	}
}

// LowPassZeroPhase applies a Butterworth low-pass in both directions, so the
// result has no phase shift and can be compared/subtracted sample-for-sample.
func LowPassZeroPhase(x []float64, sampleRate int, cutoff float64, order int) []float64 {
	return filterZeroPhase(x, butterworthSections(sampleRate, cutoff, order, false))
}

// HighPassZeroPhase applies a zero-phase Butterworth high-pass.
func HighPassZeroPhase(x []float64, sampleRate int, cutoff float64, order int) []float64 {
	return filterZeroPhase(x, butterworthSections(sampleRate, cutoff, order, true))
}

// BandPassZeroPhase applies independent zero-phase high- and low-pass stages.
func BandPassZeroPhase(x []float64, sampleRate int, lowCutoff, highCutoff float64) []float64 {
	return LowPassZeroPhase(HighPassZeroPhase(x, sampleRate, lowCutoff, 4), sampleRate, highCutoff, 4)
}

func rms(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(x)))
}

// RMSdBFS expresses RMS amplitude relative to full scale (1.0).
func RMSdBFS(x []float64) float64 {
	return 20 * math.Log10(max(rms(x), 1e-12))
}

func frameRMSdB(x []float64, start, length int) float64 {
	start = max(0, start)
	end := min(len(x), start+length)
	if end <= start {
		return -240
	}
	return RMSdBFS(x[start:end])
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	pos := p * float64(len(cp)-1)
	lo := int(math.Floor(pos))
	hi := min(lo+1, len(cp)-1)
	frac := pos - float64(lo)
	return cp[lo]*(1-frac) + cp[hi]*frac
}

func dotAt(dialogue, reference []float64, start, end, lag int) (dot, dEnergy, rEnergy float64, count int) {
	lo := max(start, lag)
	hi := min(end, len(reference)+lag)
	if hi <= lo {
		return 0, 0, 0, 0
	}
	for i := lo; i < hi; i++ {
		d := dialogue[i]
		r := reference[i-lag]
		dot += d * r
		dEnergy += d * d
		rEnergy += r * r
	}
	return dot, dEnergy, rEnergy, hi - lo
}

func bestCorrelation(dialogue, reference []float64, start, end, maxLag int) (lag int, corr float64, valid bool) {
	start = max(0, start)
	end = min(len(dialogue), end)
	if end-start < 32 {
		return 0, 0, false
	}
	best := 0.0
	bestLag := 0
	for candidate := -maxLag; candidate <= maxLag; candidate++ {
		dot, de, re, n := dotAt(dialogue, reference, start, end, candidate)
		if n < 32 || de <= 1e-18 || re <= 1e-18 {
			continue
		}
		c := dot / math.Sqrt(de*re)
		if math.Abs(c) > math.Abs(best) {
			best, bestLag = c, candidate
		}
	}
	return bestLag, best, best != 0
}

func leastSquaresGain(dialogue, reference []float64, start, end, lag int) float64 {
	dot, _, re, _ := dotAt(dialogue, reference, start, end, lag)
	if re <= 1e-18 {
		return 0
	}
	return dot / re
}

func correlationAt(dialogue, reference []float64, start, end, lag int) float64 {
	dot, de, re, n := dotAt(dialogue, reference, start, end, lag)
	if n == 0 || de <= 1e-18 || re <= 1e-18 {
		return 0
	}
	return dot / math.Sqrt(de*re)
}
