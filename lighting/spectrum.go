package lighting

import "math"

func analyzeBands(series []float64, fps float64, cfg Config) []BandStat {
	if len(series) < 4 || fps <= 0 {
		return emptyBands(cfg, max(fps/2, 0))
	}
	n := len(series)
	nyquist := fps / 2

	centered := make([]float64, n)
	mean := 0.0
	for _, v := range series {
		mean += v
	}
	mean /= float64(n)
	slope := linearSlope(series)
	for i, v := range series {
		centered[i] = v - mean - slope*float64(i)
	}

	nfft := nextPowerOfTwo(n * 4)
	if nfft > 1<<20 {
		nfft = 1 << 20
	}
	if nfft < n {
		nfft = nextPowerOfTwo(n)
	}
	re := make([]float64, nfft)
	im := make([]float64, nfft)
	windowPower := 0.0
	for i, v := range centered {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		re[i] = v * w
		windowPower += w * w
	}
	fft(re, im)

	power := make([]float64, nfft/2+1)
	total := 0.0
	for k := 1; k <= nfft/2; k++ {
		p := 2 * (re[k]*re[k] + im[k]*im[k]) / (float64(nfft) * windowPower)
		if k == nfft/2 {
			p *= 0.5
		}
		power[k] = p
		total += p
	}
	df := fps / float64(nfft)

	bounds := [][2]float64{
		{0, clamp(cfg.SlowCutoffHz, 0, nyquist)},
		{cfg.SlowCutoffHz, clamp(cfg.ExposureBandHighHz, 0, nyquist)},
		{cfg.FlickerBandLowHz, nyquist},
	}
	out := make([]BandStat, 0, len(bounds))
	for _, b := range bounds {
		lo, hi := b[0], b[1]
		if hi < lo {
			hi = lo
		}
		stat := bandPower(power, df, lo, hi, total)
		out = append(out, stat)
	}
	return out
}

func bandPower(power []float64, df, lo, hi, total float64) BandStat {
	s := BandStat{LowHz: lo, HighHz: hi}
	if len(power) < 2 || hi <= lo {
		return s
	}
	k0 := int(math.Ceil(lo / df))
	k1 := int(math.Ceil(hi/df)) - 1
	k0 = max(1, k0)
	k1 = min(len(power)-1, k1)
	if k1 < k0 {
		return s
	}
	sum, peakPower, peakBin := 0.0, -1.0, -1
	for k := k0; k <= k1; k++ {
		sum += power[k]
		if power[k] > peakPower {
			peakPower, peakBin = power[k], k
		}
	}
	s.RMSLuma = math.Sqrt(sum)
	if total > 0 {
		s.RelativePower = sum / total
	}
	if peakBin >= 0 {
		if sum > 0 {
			s.PeakConcentration = peakPower * float64(k1-k0+1) / sum
		}
		s.PeakHz = float64(peakBin) * df
		if peakBin > 1 && peakBin < len(power)-1 {
			a, b, c := math.Log(math.Max(power[peakBin-1], 1e-20)),
				math.Log(math.Max(power[peakBin], 1e-20)),
				math.Log(math.Max(power[peakBin+1], 1e-20))
			den := a - 2*b + c
			if math.Abs(den) > 1e-15 {
				delta := 0.5 * (a - c) / den
				if delta > -0.5 && delta < 0.5 {
					s.PeakHz = (float64(peakBin) + delta) * df
				}
			}
		}
		s.PeakAtEdge = peakBin == k0 && k0 > 1 || peakBin == k1 && k1 < len(power)-1
	}
	return s
}

func emptyBands(cfg Config, nyquist float64) []BandStat {
	return []BandStat{
		{LowHz: 0, HighHz: clamp(cfg.SlowCutoffHz, 0, nyquist)},
		{LowHz: cfg.SlowCutoffHz, HighHz: clamp(cfg.ExposureBandHighHz, 0, nyquist)},
		{LowHz: cfg.FlickerBandLowHz, HighHz: nyquist},
	}
}

func linearSlope(series []float64) float64 {
	n := float64(len(series))
	if n < 2 {
		return 0
	}
	meanX := (n - 1) / 2
	meanY := 0.0
	for _, v := range series {
		meanY += v
	}
	meanY /= n
	num, den := 0.0, 0.0
	for i, v := range series {
		dx := float64(i) - meanX
		num += dx * (v - meanY)
		den += dx * dx
	}
	if den == 0 {
		return 0
	}
	return num / den
}

func nextPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wr, wi := math.Cos(angle), math.Sin(angle)
		for i := 0; i < n; i += length {
			cr, ci := 1.0, 0.0
			for j := 0; j < length/2; j++ {
				a := i + j
				b := a + length/2
				tr := cr*re[b] - ci*im[b]
				ti := cr*im[b] + ci*re[b]
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}

func movingAverage(series []float64, window int) []float64 {
	n := len(series)
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	window = max(1, min(window, n))
	left := window / 2
	right := window - left - 1
	prefix := make([]float64, n+1)
	for i, v := range series {
		prefix[i+1] = prefix[i] + v
	}
	startMean := prefix[window] / float64(window)
	endMean := (prefix[n] - prefix[n-window]) / float64(window)
	for i := range series {
		if i-left < 0 {
			out[i] = startMean
			continue
		}
		if i+right >= n {
			out[i] = endMean
			continue
		}
		lo := max(0, i-left)
		hi := min(n, i+right+1)
		out[i] = (prefix[hi] - prefix[lo]) / float64(hi-lo)
	}
	return out
}
