package analyze

import (
	"clipforensic/internal/dsp"
	"math"
)

func analyzeSpectrum(x []float64, sampleRate, fftSize int) SpectrumMetrics {
	if len(x) == 0 {
		return SpectrumMetrics{FFTSize: fftSize}
	}
	window := x
	if len(window) > fftSize {
		center := len(window) / 2
		half := fftSize / 2
		lo := center - half
		hi := center + half
		if lo < 0 {
			lo = 0
		}
		if hi > len(window) {
			hi = len(window)
		}
		window = window[lo:hi]
	}
	mags, normFreqs, n := dsp.MagnitudeSpectrum(window)
	out := SpectrumMetrics{FFTSize: n}
	binHz := float64(sampleRate) / float64(n)

	var total, weighted, logSum float64
	logCount := 0
	energy := make([]float64, len(mags))
	for i, m := range mags {
		e := m * m
		energy[i] = e
		total += e
		hz := normFreqs[i] * float64(sampleRate)
		weighted += hz * e
		if m > 1e-12 {
			logSum += math.Log(m)
			logCount++
		}
		if hz >= 20 && hz < 250 {
			out.LowRatio += e
		} else if hz < 4000 {
			out.MidRatio += e
		} else if hz < 12000 {
			out.HighRatio += e
		} else {
			out.AirRatio += e
		}
	}
	if total > 0 {
		out.CentroidHz = weighted / total
		out.LowRatio /= total
		out.MidRatio /= total
		out.HighRatio /= total
		out.AirRatio /= total
		threshold := 0.85 * total
		acc := 0.0
		for i, e := range energy {
			acc += e
			if acc >= threshold {
				out.Rolloff85Hz = float64(i) * binHz
				break
			}
		}
	}
	if logCount > 0 {
		geo := math.Exp(logSum / float64(logCount))
		arith := 0.0
		for _, m := range mags {
			arith += m
		}
		arith /= float64(len(mags))
		if arith > 0 {
			out.Flatness = geo / arith
		}
	}

	f0 := estimateF0(x, sampleRate)
	out.EstimatedF0Hz = f0
	if f0 > 0 {
		harmonic := 0.0
		odd := 0.0
		even := 0.0
		for h := 1; h <= 16; h++ {
			centerBin := int(math.Round(float64(h) * f0 / binHz))
			if centerBin <= 0 || centerBin >= len(energy)-2 {
				continue
			}
			band := 0.0
			for b := centerBin - 1; b <= centerBin+1; b++ {
				if b >= 0 && b < len(energy) {
					band += energy[b]
				}
			}
			harmonic += band
			if h%2 == 1 {
				odd += band
			} else {
				even += band
			}
		}
		out.HarmonicStrength = clamp01(harmonic / total)
		if even > 1e-12 {
			out.OddEvenRatio = odd / even
		} else if odd > 0 {
			out.OddEvenRatio = 99
		}
	}
	return out
}

func estimateF0(x []float64, sampleRate int) float64 {
	if len(x) < sampleRate/20 {
		return 0
	}
	limit := len(x)
	if limit > 4096 {
		limit = 4096
	}
	minLag := sampleRate / 1200
	maxLag := sampleRate / 55
	if maxLag >= limit {
		maxLag = limit - 1
	}
	bestLag := 0
	best := 0.0
	for lag := minLag; lag <= maxLag; lag++ {
		var ab, aa, bb float64
		n := limit - lag
		for i := 0; i < n; i++ {
			a := x[i]
			b := x[i+lag]
			ab += a * b
			aa += a * a
			bb += b * b
		}
		if aa <= 1e-9 || bb <= 1e-9 {
			continue
		}
		r := ab / math.Sqrt(aa*bb)
		if r > best {
			best = r
			bestLag = lag
		}
	}
	if best < 0.35 || bestLag == 0 {
		return 0
	}
	return float64(sampleRate) / float64(bestLag)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func dbFS(v float64) float64 {
	if v <= 1e-12 {
		return -120
	}
	return 20 * math.Log10(v)
}

func bandEnergy(x []float64, sampleRate int, lowHz, highHz float64) float64 {
	if len(x) < 64 {
		return 0
	}
	mags, normFreqs, _ := dsp.MagnitudeSpectrum(x)
	energy := 0.0
	for i, m := range mags {
		hz := normFreqs[i] * float64(sampleRate)
		if hz >= lowHz && hz < highHz {
			energy += m * m
		}
	}
	return energy
}

func splatterDeltaDB(ch []float64, reg region, evidence, sampleRate int) float64 {
	const n = 1024
	eventLo := clampIndex(evidence-n/2, len(ch))
	eventHi := clampIndex(eventLo+n, len(ch))
	if eventHi-eventLo < n/2 {
		return 0
	}
	event := bandEnergy(ch[eventLo:eventHi], sampleRate, 2000, 12000)

	var baseline float64
	count := 0
	beforeHi := clampIndex(reg.Start-sampleRate/100, len(ch))
	beforeLo := clampIndex(beforeHi-n, len(ch))
	if beforeHi-beforeLo >= n/2 {
		baseline += bandEnergy(ch[beforeLo:beforeHi], sampleRate, 2000, 12000)
		count++
	}
	afterLo := clampIndex(reg.End+sampleRate/100, len(ch))
	afterHi := clampIndex(afterLo+n, len(ch))
	if afterHi-afterLo >= n/2 {
		baseline += bandEnergy(ch[afterLo:afterHi], sampleRate, 2000, 12000)
		count++
	}
	if count == 0 {
		return 0
	}
	baseline /= float64(count)
	if baseline <= 1e-12 {
		if event <= 1e-12 {
			return 0
		}
		return 30
	}
	return clampSplatter(10 * math.Log10((event+1e-12)/(baseline+1e-12)))
}

func clampSplatter(v float64) float64 {
	return math.Max(-24, math.Min(30, v))
}
