package lighting

import "math"

func flickerFromBands(bands Bands, cfg Config) FlickerEvidence {
	ev := FlickerEvidence{}
	if len(bands.Background) < 3 || len(bands.Motion) < 3 {
		return ev
	}
	bg := bands.Background[2]
	mot := bands.Motion[2]
	ev.BackgroundRMS = bg.RMSLuma
	ev.BackgroundRelative = bg.RelativePower
	ev.PeakHz = bg.PeakHz
	ev.PeakConcentration = bg.PeakConcentration
	if bg.RMSLuma > 1e-9 {
		ev.MotionBandRatio = mot.RMSLuma / bg.RMSLuma
	}
	relativeOK := bg.RelativePower >= cfg.FlickerMinRelative
	strongPeak := bg.RMSLuma >= 2*cfg.FlickerMinRMS &&
		bg.PeakConcentration >= 2*cfg.FlickerMinPeak
	ev.Periodic = bg.RMSLuma >= cfg.FlickerMinRMS &&
		(relativeOK || strongPeak) &&
		bg.PeakConcentration >= cfg.FlickerMinPeak &&
		bg.PeakHz > cfg.SlowCutoffHz
	return ev
}

func flickerScore(bands Bands, ev FlickerEvidence, cfg Config) float64 {
	if !ev.Periodic || len(bands.Background) < 3 || len(bands.Motion) < 3 {
		return 0
	}
	rmsScore := clamp(ev.BackgroundRMS/(cfg.FlickerMinRMS*2), 0, 1)
	relScore := clamp((ev.BackgroundRelative-cfg.FlickerMinRelative/2)/
		(1-cfg.FlickerMinRelative/2), 0, 1)
	peakScore := clamp(ev.PeakConcentration/(cfg.FlickerMinPeak*3), 0, 1)
	indepScore := clamp(1-ev.MotionBandRatio, 0, 1)
	return clamp(0.35*rmsScore+0.30*relScore+0.20*peakScore+0.15*indepScore, 0, 1)
}

func classifyReport(events []Event, flicker FlickerEvidence, ledScore float64,
	cfg Config) Classification {
	var ae, cloud *Event
	for i := range events {
		switch events[i].Kind {
		case "auto_exposure":
			if ae == nil || events[i].Confidence > ae.Confidence {
				ae = &events[i]
			}
		case "cloud_shadow":
			if cloud == nil || events[i].Confidence > cloud.Confidence {
				cloud = &events[i]
			}
		}
	}
	led := flicker.Periodic && ledScore >= 0.50
	reasons := make([]string, 0, 4)
	if led {
		reasons = append(reasons,
			"stable-background energy is periodic in the flicker band",
			"foreground-motion evidence does not explain the periodic component")
	}
	hasAE := ae != nil && math.Abs(ae.Amplitude) >= cfg.EventMinAmplitude
	hasCloud := cloud != nil && math.Abs(cloud.Amplitude) >= cfg.EventMinAmplitude
	if hasAE {
		reasons = append(reasons, "a short global change is regionally uniform")
	}
	if hasCloud {
		reasons = append(reasons, "a sustained background change remains in the slow curve")
	}

	out := Classification{Primary: "stable"}
	switch {
	case led && (hasAE || hasCloud):
		out.Primary = "mixed"
		out.Confidence = clamp(0.55+0.25*ledScore, 0, 0.98)
	case led:
		out.Primary = "led_flicker"
		out.Confidence = ledScore
	case hasAE && hasCloud:
		out.Primary = "mixed"
		out.Confidence = clamp((ae.Confidence+cloud.Confidence)/2, 0, 0.98)
	case hasAE:
		out.Primary = "auto_exposure"
		out.Confidence = ae.Confidence
	case hasCloud:
		out.Primary = "cloud_shadow"
		out.Confidence = cloud.Confidence
	default:
		out.Confidence = 0.65
		reasons = append(reasons, "no periodic or sustained change exceeds the evidence thresholds")
	}
	out.Reasons = reasons
	return out
}

func reportWarnings(a *Analyzer, fps float64, n int, stableMean float64) []string {
	var warnings []string
	if n < 8 {
		warnings = append(warnings, "too few frames for a stable spectrum")
	}
	if fps <= 0 {
		warnings = append(warnings, "frame rate could not be inferred")
	} else {
		if n > 2 {
			intervals := make([]float64, n-1)
			for i := 1; i < n; i++ {
				intervals[i-1] = a.samples[i].timeS - a.samples[i-1].timeS
			}
			mean := meanSlice(intervals)
			if mean > 0 && stddev(intervals)/mean > 0.01 {
				warnings = append(warnings, "frame timestamps are irregular; FFT bands assume uniform sampling")
			}
		}
		if fps/2 < a.cfg.ExposureBandHighHz {
			warnings = append(warnings, "Nyquist frequency is below the exposure/flicker split")
		}
		warnings = append(warnings,
			"frequencies above Nyquist are aliased; this cannot identify an unseen true LED rate")
	}
	if stableMean < 0.20 {
		warnings = append(warnings, "few stable background blocks; background evidence is weak")
	}
	return warnings
}

func bandRMS(bands []BandStat, index int) float64 {
	if index < 0 || index >= len(bands) {
		return 0
	}
	return bands[index].RMSLuma
}

func rangeOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return maxOf(values) - minOf(values)
}

func minOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := values[0]
	for _, x := range values[1:] {
		if x < v {
			v = x
		}
	}
	return v
}

func maxOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := values[0]
	for _, x := range values[1:] {
		if x > v {
			v = x
		}
	}
	return v
}

func maxChangeRate(timeS, values []float64) float64 {
	best := 0.0
	for i := 1; i < len(values) && i < len(timeS); i++ {
		dt := timeS[i] - timeS[i-1]
		if dt > 0 {
			best = math.Max(best, math.Abs(values[i]-values[i-1])/dt)
		}
	}
	return best
}
