package click

import "math"

func analyticPhase(y []float64) ([]float64, []float64) {
	const taps = 31
	const delay = taps / 2
	h := make([]float64, taps)
	for k := range h {
		off := k - delay
		if off&1 != 0 {
			h[k] = 2 / (math.Pi * float64(off))
		}
	}
	phase := make([]float64, len(y))
	amp := make([]float64, len(y))
	for n := taps - 1; n < len(y); n++ {
		var imag float64
		for k := 0; k < taps; k++ {
			imag += h[k] * y[n-k]
		}
		real := y[n-delay]
		phase[n] = math.Atan2(imag, real)
		amp[n] = math.Hypot(real, imag)
	}
	return phase, amp
}

func evaluateCandidate(ch *channelData, cand candidate, sampleRate int) featureSet {
	n := len(ch.x)
	sample := cand.sample
	if sample < 1 || sample >= n {
		return featureSet{}
	}
	boundary := sample
	searchSec := 0.0006
	if cand.lowFreq {
		searchSec = 0.0015
	}
	search := int(math.Round(float64(sampleRate) * searchSec))
	if search < 1 {
		search = 1
	}
	bestJump := boundary
	for i := maxInt(1, sample-search); i <= minInt(n-1, sample+search); i++ {
		if ch.jump[i] > ch.jump[bestJump] {
			bestJump = i
		}
	}
	for back := 1; back <= 3 && bestJump-back >= 1; back++ {
		if ch.jump[bestJump-back] >= 0.5*ch.jump[bestJump] {
			bestJump -= back
			break
		}
	}
	_, sampleP99 := localLimit(ch.jumpProf, sample, sampleRate)
	if !cand.lowFreq || ch.jump[bestJump] >= 2.0*math.Max(sampleP99, 1e-7) {
		if !cand.lowFreq || ch.jump[bestJump] > 1e-4 {
			boundary = bestJump
		}
	}

	f := Features{JumpValue: ch.jump[boundary]}
	_, localP99 := localLimit(ch.jumpProf, boundary, sampleRate)
	f.JumpRatio = f.JumpValue / math.Max(localP99, 1e-7)
	f.JumpZ = robustZ(ch.jump, boundary, sampleRate, 0.020, 0.0008)
	f.SlopeJumpZ = robustZ(ch.slope, boundary, sampleRate, 0.020, 0.0008)
	f.PhaseJumpRad, f.PhaseCoherence = phaseStep(ch.x, boundary, sampleRate)
	f.LowFreqStep, f.LowFreqStepZ = lowFrequencyStep(ch.lowPass, boundary, sampleRate)
	f.LowFreqImpulse, f.LowFreqImpulseZ = lowFrequencyImpulse(ch.lowPass, boundary, sampleRate)
	f.BurstSamples = burstCount(ch.jump, boundary, sampleRate, localP99)

	conf := confidenceFor(f)
	set := featureSet{sample: boundary, features: f, confidence: conf}
	percussive := percussiveOnset(ch.x, boundary, sampleRate)
	f.PercussiveOnset = percussive
	set.features = f

	if percussive {
		set.suppressed = true
		set.reason = "percussive onset with sustained transient; kept as normal drum/attack"
	}
	if f.BurstSamples >= 10 && f.JumpRatio < 20 {
		set.suppressed = true
		set.reason = "sustained broadband attack rather than isolated discontinuity"
	}
	return set
}

func robustZ(v []float64, center, sampleRate int, contextSec, guardSec float64) float64 {
	ctx := int(math.Round(float64(sampleRate) * contextSec))
	guard := int(math.Round(float64(sampleRate) * guardSec))
	lo := maxInt(1, center-ctx)
	hi := minInt(len(v), center+ctx+1)
	buf := make([]float64, 0, hi-lo)
	for i := lo; i < hi; i++ {
		if i >= center-guard && i <= center+guard {
			continue
		}
		buf = append(buf, v[i])
	}
	scale := madScale(buf)
	return v[center] / math.Max(scale, 1e-9)
}

func phaseStep(x []float64, sample, sampleRate int) (jumpRad, coherence float64) {
	win := int(math.Round(float64(sampleRate) * 0.004))
	gap := int(math.Round(float64(sampleRate) * 0.0004))
	warmup := int(math.Round(float64(sampleRate) * 0.020))
	if win < 8 {
		win = 8
	}
	if gap < 1 {
		gap = 1
	}
	fs := float64(sampleRate)
	for _, r := range [][2]float64{{200, 1000}, {800, 3000}, {2500, 8000}} {
		if r[0] >= fs*0.44 {
			continue
		}
		hi := math.Min(r[1], fs*0.45)
		if hi <= r[0] {
			continue
		}
		preLo, preHi := sample-gap-win, sample-gap
		postLo, postHi := sample+gap, sample+gap+win
		if preLo < 0 || postHi > len(x) {
			continue
		}
		start := maxInt(0, preLo-warmup)
		seg := filter(x[start:postHi], newBandpass(fs, r[0], hi))
		phase, amp := analyticPhase(seg)
		preLo, preHi = preLo-start, preHi-start
		postLo, postHi = postLo-start, postHi-start
		if preLo < 0 || postHi > len(phase) {
			continue
		}
		if rms(amp[preLo:preHi]) < 1e-5 || rms(amp[postLo:postHi]) < 1e-5 {
			continue
		}
		boundary := sample - start
		prePhase, preResidual := fitPhase(phase, preLo, preHi, boundary)
		postPhase, postResidual := fitPhase(phase, postLo, postHi, boundary)
		coh := 1 - math.Max(preResidual, postResidual)/0.9
		coh = clamp(coh, 0, 1)
		if coh < 0.30 {
			continue
		}
		jump := math.Abs(wrapPhase(postPhase - prePhase))
		if jump > jumpRad {
			jumpRad = jump
			coherence = coh
		}
	}
	return jumpRad, coherence
}

func fitPhase(phase []float64, lo, hi, boundary int) (atBoundary, residualRMS float64) {
	if hi-lo < 4 {
		return 0, math.Pi
	}
	unwrapped := make([]float64, hi-lo)
	unwrapped[0] = phase[lo]
	for i := 1; i < len(unwrapped); i++ {
		unwrapped[i] = unwrapped[i-1] + wrapPhase(phase[lo+i]-phase[lo+i-1])
	}
	var sx, sy, sxx, sxy float64
	n := float64(len(unwrapped))
	for i, y := range unwrapped {
		x := float64(lo+i) - float64(boundary)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	den := n*sxx - sx*sx
	slope := 0.0
	if math.Abs(den) > 1e-12 {
		slope = (n*sxy - sx*sy) / den
	}
	intercept := (sy - slope*sx) / n
	var errSum float64
	for i, y := range unwrapped {
		x := float64(lo+i) - float64(boundary)
		d := y - (intercept + slope*x)
		errSum += d * d
	}
	return intercept, math.Sqrt(errSum / n)
}

func lowFrequencyStep(lf []float64, center, sampleRate int) (step, z float64) {
	win := int(math.Round(float64(sampleRate) * 0.008))
	gap := int(math.Round(float64(sampleRate) * 0.001))
	if win < 4 {
		win = 4
	}
	preStart, preEnd := maxInt(0, center-gap-win), maxInt(0, center-gap)
	postStart, postEnd := minInt(len(lf), center+gap), minInt(len(lf), center+gap+win)
	if preEnd <= preStart || postEnd <= postStart {
		return 0, 0
	}
	step = math.Abs(mean(lf[postStart:postEnd]) - mean(lf[preStart:preEnd]))
	scale := lowFrequencyVariation(lf, center, sampleRate)
	return step, step / math.Max(scale, 1e-8)
}

func lowFrequencyImpulse(lf []float64, center, sampleRate int) (peak, z float64) {
	ctx := lowFreqContext(lf, center, sampleRate)
	base := median(ctx)
	radius := int(math.Round(float64(sampleRate) * 0.004))
	lo, hi := maxInt(0, center-radius), minInt(len(lf), center+radius+1)
	for _, x := range lf[lo:hi] {
		if d := math.Abs(x - base); d > peak {
			peak = d
		}
	}
	scale := lowFrequencyVariation(lf, center, sampleRate)
	return peak, peak / math.Max(scale, 1e-8)
}

func lowFrequencyVariation(lf []float64, center, sampleRate int) float64 {
	ctx := int(math.Round(float64(sampleRate) * float64(2) * contextSeconds))
	gap := int(math.Round(float64(sampleRate) * 0.002))
	preLo, preHi := maxInt(0, center-ctx), maxInt(0, center-gap)
	postLo, postHi := minInt(len(lf), center+gap), minInt(len(lf), center+ctx+1)
	var scales []float64
	if preHi-preLo > 8 {
		scales = append(scales, madScale(lf[preLo:preHi]))
	}
	if postHi-postLo > 8 {
		scales = append(scales, madScale(lf[postLo:postHi]))
	}
	if len(scales) == 0 {
		return 0
	}
	return math.Max(scales[0], scales[len(scales)-1])
}

func lowFreqContext(lf []float64, center, sampleRate int) []float64 {
	ctx := int(math.Round(float64(sampleRate) * float64(2) * contextSeconds))
	lo, hi := maxInt(0, center-ctx), minInt(len(lf), center+ctx+1)
	return lf[lo:hi]
}

func burstCount(jump []float64, center, sampleRate int, localP99 float64) int {
	radius := int(math.Round(float64(sampleRate) * 0.005))
	peak := 0.0
	for i := maxInt(1, center-radius); i <= minInt(len(jump)-1, center+radius); i++ {
		peak = math.Max(peak, jump[i])
	}
	limit := math.Max(4*localP99, 0.25*peak)
	limit = math.Max(limit, 1e-6)
	count := 0
	for i := maxInt(1, center-radius); i <= minInt(len(jump)-1, center+radius); i++ {
		if jump[i] >= limit {
			count++
		}
	}
	return count
}

func percussiveOnset(x []float64, center, sampleRate int) bool {
	gap := int(math.Round(float64(sampleRate) * 0.001))
	short := int(math.Round(float64(sampleRate) * 0.015))
	peakWin := int(math.Round(float64(sampleRate) * 0.002))
	preLo, preHi := maxInt(0, center-gap-short), maxInt(0, center-gap)
	postLo, postHi := minInt(len(x), center+gap), minInt(len(x), center+gap+short)
	peakLo, peakHi := maxInt(0, center-peakWin), minInt(len(x), center+gap+peakWin)
	if preHi-preLo < 4 || postHi-postLo < 4 {
		return false
	}
	pre := rms(x[preLo:preHi])
	post := rms(x[postLo:postHi])
	peak := rms(x[peakLo:peakHi])
	tailLo := minInt(len(x), center+gap+peakWin)
	tailHi := minInt(len(x), center+gap+short)
	if tailHi-tailLo < 4 {
		return false
	}
	tail := rms(x[tailLo:tailHi])
	return post > math.Max(4*pre, 0.001) && tail > math.Max(0.10*peak, 0.0005)
}

func confidenceFor(f Features) float64 {
	jumpScore := ramp(math.Log2(math.Max(f.JumpRatio, 1)), 1.35, 4.0)
	isolation := 0.0
	switch {
	case f.BurstSamples <= 2:
		isolation = 1
	case f.BurstSamples <= 4:
		isolation = 0.55
	case f.BurstSamples <= 8:
		isolation = 0.18
	}
	phaseScore := ramp(f.PhaseJumpRad, 0.15, 1.5) * ramp(f.PhaseCoherence, 0.25, 0.70)
	lfScore := math.Max(
		ramp(f.LowFreqStepZ, 1.8, 4.5),
		ramp(f.LowFreqImpulseZ, 2.0, 5.0),
	)
	slopeScore := ramp(math.Log2(math.Max(f.SlopeJumpZ, 1)), 1.8, 4.5)
	conf := 0.40*jumpScore + 0.14*isolation + 0.20*phaseScore + 0.14*lfScore + 0.12*slopeScore
	if lfScore >= 0.50 {
		conf = math.Max(conf, 0.38+0.35*lfScore)
	}
	if f.BurstSamples <= 2 && f.PhaseCoherence >= 0.60 && phaseScore >= 0.50 && slopeScore >= 0.60 {
		conf = math.Max(conf, 0.62)
	}
	if f.BurstSamples <= 2 && f.JumpRatio >= 8 {
		conf = math.Max(conf, 0.70)
	}
	return clamp(conf, 0, 0.99)
}

func flagsFor(f Features) []string {
	var flags []string
	if f.JumpRatio >= 2.2 && f.JumpValue >= 1e-4 {
		flags = append(flags, "sample_discontinuity")
	}
	if f.PhaseJumpRad >= 0.40 && f.PhaseCoherence >= 0.30 {
		flags = append(flags, "phase_jump")
	}
	if f.LowFreqStepZ >= 3.0 || f.LowFreqImpulseZ >= 4.2 {
		flags = append(flags, "low_frequency_impulse")
	}
	return flags
}

func ramp(v, lo, hi float64) float64 {
	if hi <= lo {
		return 0
	}
	return clamp((v-lo)/(hi-lo), 0, 1)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
