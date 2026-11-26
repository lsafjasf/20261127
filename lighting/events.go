package lighting

import (
	"math"
	"sort"
)

type eventGroup struct {
	start, end int
	sign       int
	peakSlope  float64
}

func detectLightEvents(samples []frameSample, timeS, sustained []float64,
	regions [][]float64, fps float64, cfg Config, flicker FlickerEvidence) []Event {
	n := len(sustained)
	if n < 2 || fps <= 0 {
		return nil
	}
	half := max(1, int(math.Round(0.5*fps)))
	slopes := make([]float64, n)
	active := make([]bool, n)
	for i := 0; i < n; i++ {
		lo, hi := max(0, i-half), min(n-1, i+half)
		dt := timeS[hi] - timeS[lo]
		if dt > 0 {
			slopes[i] = (sustained[hi] - sustained[lo]) / dt
		}
		active[i] = math.Abs(slopes[i]) >= cfg.EventSlopeThreshold
	}

	gap := max(1, int(math.Round(0.75*fps)))
	var groups []eventGroup
	for i := 0; i < n; {
		if !active[i] {
			i++
			continue
		}
		start, end := i, i
		j := i + 1
		for j < n {
			if active[j] {
				end = j
				j++
				continue
			}
			k := j
			for k < n && k-j <= gap && !active[k] {
				k++
			}
			if k < n && active[k] && k-j <= gap {
				end = k
				j = k + 1
				continue
			}
			break
		}
		for start > 0 && active[start-1] && sameSign(slopes[start-1], slopes[start]) {
			start--
		}
		for end+1 < n && active[end+1] && sameSign(slopes[end+1], slopes[end]) {
			end++
		}
		g := eventGroup{start: start, end: end}
		for j := start; j <= end; j++ {
			g.sign += sign(slopes[j])
			g.peakSlope = math.Max(g.peakSlope, math.Abs(slopes[j]))
		}
		if g.sign == 0 {
			g.sign = sign(slopes[(start+end)/2])
		}
		groups = append(groups, g)
		i = end + 1
	}
	groups = mergeEventGroups(groups, max(2, int(math.Round(1.2*fps))))

	var events []Event
	for _, g := range groups {
		ev, ok := measureEvent(samples, timeS, sustained, regions, g, fps, cfg, flicker)
		if ok {
			events = append(events, ev)
		}
	}
	if len(events) == 0 {
		if ev, ok := rangeEvent(samples, timeS, sustained, regions, fps, cfg, flicker); ok {
			events = append(events, ev)
		}
	}
	return events
}

func mergeEventGroups(groups []eventGroup, maxGap int) []eventGroup {
	if len(groups) < 2 {
		return groups
	}
	out := []eventGroup{groups[0]}
	for _, g := range groups[1:] {
		last := &out[len(out)-1]
		if sameSign(float64(g.sign), float64(last.sign)) && g.start-last.end <= maxGap {
			last.end = g.end
			last.sign += g.sign
			last.peakSlope = math.Max(last.peakSlope, g.peakSlope)
			continue
		}
		out = append(out, g)
	}
	return out
}

func measureEvent(samples []frameSample, timeS, sustained []float64,
	regions [][]float64, g eventGroup, fps float64, cfg Config,
	flicker FlickerEvidence) (Event, bool) {
	n := len(sustained)
	settle := max(1, int(math.Round(0.5*fps)))
	before := meanSlice(sustained[max(0, g.start-settle) : g.start+1])
	after := meanSlice(sustained[g.end:min(n, g.end+settle+1)])
	amplitude := after - before
	if math.Abs(amplitude) < cfg.EventMinAmplitude {
		return Event{}, false
	}
	duration := timeS[g.end] - timeS[g.start]
	if duration <= 0 {
		duration = float64(g.end-g.start+1) / fps
	}
	uniformity, coverage, phaseSpread := regionEventStats(
		timeS, regions, g.start, g.end, amplitude, fps)
	motion, stable := meanMotionAndStable(samples, g.start, g.end)
	kind, confidence := classifyEvent(duration, g.peakSlope, uniformity,
		coverage, phaseSpread, flicker)
	return Event{
		Kind:       kind,
		StartS:     timeS[g.start],
		EndS:       timeS[g.end],
		Amplitude:  amplitude,
		Confidence: confidence,
		Retained:   true,
		Evidence: EventEvidence{
			DurationS:          duration,
			AmplitudeLuma:      amplitude,
			PeakSlopeLumaS:     g.peakSlope,
			BackgroundFraction: stable,
			MotionFraction:     motion,
			RegionUniformity:   uniformity,
			RegionCoverage:     coverage,
			RegionPhaseSpreadS: phaseSpread,
			FlickerRelative:    flicker.BackgroundRelative,
		},
	}, true
}

func classifyEvent(duration, peakSlope, uniformity, coverage, phaseSpread float64,
	flicker FlickerEvidence) (string, float64) {
	uniformShort := uniformity >= 0.60 && coverage >= 0.70 &&
		phaseSpread <= 0.45
	if uniformShort && (duration <= 2.5 || peakSlope >= 4.0) {
		c := 0.48 + 0.18*uniformity + 0.12*coverage +
			0.12*clamp((2.5-duration)/2.5, 0, 1) +
			0.08*clamp((0.45-phaseSpread)/0.45, 0, 1)
		if peakSlope >= 1.0 {
			c += 0.08
		}
		if flicker.Periodic {
			c -= 0.10
		}
		return "auto_exposure", clamp(c, 0, 0.98)
	}
	if duration >= 2.0 || !uniformShort || phaseSpread > 0.55 {
		c := 0.50 + 0.20*clamp((duration-1.5)/4, 0, 1) +
			0.15*clamp(1-uniformity, 0, 1) +
			0.10*clamp(phaseSpread/0.8, 0, 1)
		return "cloud_shadow", clamp(c, 0, 0.98)
	}
	if peakSlope >= 1.2 {
		return "auto_exposure", 0.62
	}
	return "sustained_light_change", 0.55
}

func rangeEvent(samples []frameSample, timeS, sustained []float64,
	regions [][]float64, fps float64, cfg Config,
	flicker FlickerEvidence) (Event, bool) {
	n := len(sustained)
	if n < 2 {
		return Event{}, false
	}
	minI, maxI := 0, 0
	for i, v := range sustained {
		if v < sustained[minI] {
			minI = i
		}
		if v > sustained[maxI] {
			maxI = i
		}
	}
	amplitude := sustained[maxI] - sustained[minI]
	if math.Abs(amplitude) < cfg.EventMinAmplitude*1.4 {
		return Event{}, false
	}
	start, end := minI, maxI
	if start > end {
		start, end = end, start
	}
	settle := max(1, int(math.Round(0.5*fps)))
	before := meanSlice(sustained[max(0, start-settle) : start+1])
	after := meanSlice(sustained[end:min(n, end+settle+1)])
	amp := after - before
	if math.Abs(amp) < cfg.EventMinAmplitude {
		amp = amplitude
	}
	uniformity, coverage, phase := regionEventStats(timeS, regions, start, end, amp, fps)
	kind, confidence := classifyEvent(timeS[end]-timeS[start], 0,
		uniformity, coverage, phase, flicker)
	if kind == "auto_exposure" {
		kind, confidence = "sustained_light_change", 0.50
	}
	motion, stable := meanMotionAndStable(samples, start, end)
	return Event{
		Kind:       kind,
		StartS:     timeS[start],
		EndS:       timeS[end],
		Amplitude:  amp,
		Confidence: confidence,
		Retained:   true,
		Evidence: EventEvidence{
			DurationS:          timeS[end] - timeS[start],
			AmplitudeLuma:      amp,
			BackgroundFraction: stable,
			MotionFraction:     motion,
			RegionUniformity:   uniformity,
			RegionCoverage:     coverage,
			RegionPhaseSpreadS: phase,
			FlickerRelative:    flicker.BackgroundRelative,
		},
	}, true
}

func regionEventStats(timeS []float64, regions [][]float64, start, end int,
	globalAmplitude, fps float64) (uniformity, coverage, phaseSpread float64) {
	if len(regions) == 0 || len(timeS) == 0 {
		return 1, 1, 0
	}
	n := len(timeS)
	half := max(1, int(math.Round(0.5*fps)))
	preLo, preHi := max(0, start-half), start+1
	postLo, postHi := end, min(n, end+half+1)
	if postLo >= n {
		postLo = max(0, n-half)
	}
	if postLo >= postHi {
		return 0, 0, 0
	}
	amps := make([]float64, 0, len(regions))
	crossings := make([]float64, 0, len(regions))
	for _, series := range regions {
		if len(series) != n {
			continue
		}
		before := meanSlice(series[preLo:preHi])
		after := meanSlice(series[postLo:postHi])
		amp := after - before
		amps = append(amps, amp)
		if math.Abs(amp) >= 0.8 {
			target := before + 0.5*amp
			crossings = append(crossings, firstCrossing(timeS, series, target,
				max(0, start-half), min(n-1, end+half)))
		}
	}
	if len(amps) == 0 {
		return 0, 0, 0
	}
	expected := globalAmplitude
	if math.Abs(expected) < 0.5 {
		expected = medianAbs(amps)
	}
	sameSignCount := 0
	deviations := make([]float64, 0, len(amps))
	for _, amp := range amps {
		if expected != 0 && amp*expected > 0 && math.Abs(amp) >= 0.20*math.Abs(expected) {
			sameSignCount++
		}
		deviations = append(deviations, math.Abs(amp-expected))
	}
	coverage = float64(sameSignCount) / float64(len(amps))
	mad := medianAbs(deviations)
	uniformity = clamp(1-2.2*mad/math.Max(math.Abs(expected), 1), 0, 1)
	if len(crossings) >= 2 {
		phaseSpread = stddev(crossings)
	}
	return uniformity, coverage, phaseSpread
}

func firstCrossing(timeS, series []float64, target float64, lo, hi int) float64 {
	lo = max(0, lo)
	hi = min(len(series)-1, hi)
	for i := lo + 1; i <= hi; i++ {
		a, b := series[i-1], series[i]
		if (a-target)*(b-target) > 0 || a == b {
			continue
		}
		f := clamp((target-a)/(b-a), 0, 1)
		return timeS[i-1] + f*(timeS[i]-timeS[i-1])
	}
	return timeS[hi]
}

func meanMotionAndStable(samples []frameSample, start, end int) (motion, stable float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	start = max(0, start)
	end = min(len(samples)-1, end)
	if end < start {
		return 0, 0
	}
	for i := start; i <= end; i++ {
		motion += samples[i].motionFraction
		stable += samples[i].stableFraction
	}
	n := float64(end - start + 1)
	return motion / n, stable / n
}

func meanSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func medianAbs(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyVals := append([]float64(nil), values...)
	for i, v := range copyVals {
		copyVals[i] = math.Abs(v)
	}
	sort.Float64s(copyVals)
	return copyVals[len(copyVals)/2]
}

func stddev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := meanSlice(values)
	sum := 0.0
	for _, v := range values {
		d := v - mean
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(values)))
}

func corr(a, b []float64) float64 {
	n := min(len(a), len(b))
	if n < 2 {
		return 0
	}
	ma, mb := meanSlice(a[:n]), meanSlice(b[:n])
	va, vb, cov := 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		da, db := a[i]-ma, b[i]-mb
		cov += da * db
		va += da * da
		vb += db * db
	}
	if va < 1e-12 || vb < 1e-12 {
		return 0
	}
	return clamp(cov/math.Sqrt(va*vb), -1, 1)
}

func sameSign(a, b float64) bool {
	return a == 0 || b == 0 || (a > 0) == (b > 0)
}

func sign(v float64) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}
