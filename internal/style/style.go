// Package style detects shots whose black floor was lifted intentionally
// (creative milky-black grading) so compensation can be skipped or reduced.
package style

import (
	"math"
	"sort"
)

// Features are the per-frame signals used by the gate.
type Features struct {
	GlobalP1   float64 // 1st percentile of luma
	GlobalP5   float64 // 5th percentile of luma
	GlobalMin  float64 // absolute luma minimum
	ZeroFrac   float64 // fraction of pixels at or below 1/255
	NoiseSigma float64 // frame-level noise estimate
	MaskSpread float64 // corner-center spread of fitted mask
	DarkFrac   float64 // fraction of pixels below 0.10 luma
	DarkMean   float64 // mean luma of the darkest 5%
}

// Params controls the intentional-lift classifier. All levels are in [0,1].
type Params struct {
	// MinLift: global 1st percentile must be at least this high.
	MinLift float64
	// MaxZeroFrac: lifted grades almost never contain true zero pixels.
	MaxZeroFrac float64
	// MaxNoise: heavy low-light noise indicates an accidental fog on a
	// real night shot rather than a clean graded lift.
	MaxNoise float64
	// MaxMaskSpread: grading lifts are uniform, unlike cropped lens masks.
	MaxMaskSpread float64
}

// DefaultParams returns conservative thresholds (values are 8-bit levels /255).
func DefaultParams() Params {
	return Params{
		MinLift:       12.0 / 255.0,
		MaxZeroFrac:   0.002,
		MaxNoise:      4.0 / 255.0,
		MaxMaskSpread: 3.0 / 255.0,
	}
}

// ExtractFeatures computes the style features for one luma plane.
func ExtractFeatures(y []float64, sigma float64, spread float64) Features {
	cp := append([]float64(nil), y...)
	sort.Float64s(cp)
	n := len(cp)
	f := Features{
		GlobalP1:   pct(cp, 0.01),
		GlobalP5:   pct(cp, 0.05),
		GlobalMin:  cp[0],
		NoiseSigma: sigma,
		MaskSpread: spread,
	}
	darkCount := max(1, n/20)
	darkSum := 0.0
	for i := 0; i < darkCount; i++ {
		darkSum += cp[i]
	}
	for _, v := range cp {
		if v <= 1.0/255.0 {
			f.ZeroFrac++
		}
		if v < 0.10 {
			f.DarkFrac++
		}
	}
	f.ZeroFrac = f.ZeroFrac / float64(n)
	f.DarkFrac = f.DarkFrac / float64(n)
	f.DarkMean = darkSum / float64(darkCount)
	return f
}

// Score returns 0..1 confidence that the frame is an intentional lifted-black
// grade. Each rule contributes a soft membership.
func Score(f Features, p Params) float64 {
	s := 0.0
	s += ramp(f.GlobalP1, p.MinLift*0.6, p.MinLift*1.4)
	s += 1 - ramp(f.ZeroFrac, p.MaxZeroFrac*0.5, p.MaxZeroFrac*2)
	s += 1 - ramp(f.NoiseSigma, p.MaxNoise*0.6, p.MaxNoise*1.5)
	s += 1 - ramp(f.MaskSpread, p.MaxMaskSpread*0.5, p.MaxMaskSpread*2)
	return s / 4
}

// Classify applies the score threshold.
func Classify(f Features, p Params, threshold float64) (bool, float64) {
	s := Score(f, p)
	return s >= threshold, s
}

// TemporalVote smooths frame scores over a centered window and returns a
// per-frame boolean: a shot is treated as intentionally lifted only when the
// majority of the window crosses the threshold (kills single-frame flicker).
func TemporalVote(scores []float64, threshold float64, window int) []bool {
	if window < 1 {
		window = 1
	}
	r := window / 2
	out := make([]bool, len(scores))
	for i := range scores {
		yes, total := 0, 0
		for k := i - r; k <= i+r; k++ {
			if k < 0 || k >= len(scores) {
				continue
			}
			total++
			if scores[k] >= threshold {
				yes++
			}
		}
		out[i] = yes*2 > total
	}
	return out
}

func pct(sorted []float64, q float64) float64 {
	idx := q * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[lo] + (sorted[hi]-sorted[lo])*(idx-float64(lo))
}

// ramp maps x from [lo,hi] to [0,1] with hard clamps.
func ramp(x, lo, hi float64) float64 {
	if hi <= lo {
		if x >= hi {
			return 1
		}
		return 0
	}
	v := (x - lo) / (hi - lo)
	return math.Max(0, math.Min(1, v))
}
