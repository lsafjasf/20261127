package main

import (
	"math"
	"math/cmplx"
	"time"
)

type bandDef struct {
	name      string
	low       float64
	high      float64
	vocalBand bool
}

func measureFrame(left, right []float64, sampleRate, index, startSample int) FrameMetrics {
	windowedLeft, windowedRight := applyHann(left, right)
	spectrumLeft := fft(toComplex(windowedLeft))
	spectrumRight := fft(toComplex(windowedRight))

	frame := FrameMetrics{
		Index:            index,
		StartSample:      startSample,
		SampleRate:       sampleRate,
		WindowSize:       len(windowedLeft),
		StartTime:        time.Duration(float64(startSample) / float64(sampleRate) * float64(time.Second)),
		StartTimeSeconds: float64(startSample) / float64(sampleRate),
		LeftRMSDB:        rmsDB(left),
		RightRMSDB:       rmsDB(right),
		Bands:            make([]BandMetrics, 0, 9),
	}

	frame.Correlation = pearson(left, right)
	mono := sumSignals(left, right, 0.5)
	frame.MonoRMSDB = rmsDB(mono)
	frame.CancellationDB = frame.MonoRMSDB - math.Max(frame.LeftRMSDB, frame.RightRMSDB)

	side := differenceSignal(left, right)
	mid := mono
	frame.SideMidRatioDB = rmsDB(side) - rmsDB(mid)
	frame.Coherence = estimateCoherence(windowedLeft, windowedRight, sampleRate)

	bands := frequencyBands()
	var vocalPhaseX, vocalPhaseY, vocalCoherence, vocalCancellation, vocalWeight float64
	for _, band := range bands {
		metrics := measureBand(spectrumLeft, spectrumRight, frame.Coherence, band, sampleRate)
		frame.Bands = append(frame.Bands, metrics)
		if band.vocalBand && metrics.Coherence > 0 {
			weight := metrics.Coherence * math.Max(0, 1-math.Abs(metrics.MagnitudeBalance))
			vocalWeight += weight
			vocalCoherence += metrics.Coherence * weight
			vocalCancellation += metrics.Cancellation * weight
			vocalPhaseX += math.Cos(metrics.WeightedPhase) * weight
			vocalPhaseY += math.Sin(metrics.WeightedPhase) * weight
		}
	}
	if vocalWeight > 0 {
		frame.VocalWeight = vocalWeight
		frame.VocalCoherence = vocalCoherence / vocalWeight
		frame.VocalCancellation = vocalCancellation / vocalWeight
		frame.VocalPhaseRadians = math.Atan2(vocalPhaseY/vocalWeight, vocalPhaseX/vocalWeight)
	} else {
		frame.VocalPhaseRadians = 0
	}
	return frame
}

func measurePaddedFrame(left, right []float64, sampleRate, windowSize int) FrameMetrics {
	paddedLeft := make([]float64, windowSize)
	paddedRight := make([]float64, windowSize)
	copy(paddedLeft, left)
	copy(paddedRight, right)
	return measureFrame(paddedLeft, paddedRight, sampleRate, 0, 0)
}

func applyHann(left, right []float64) ([]float64, []float64) {
	n := len(left)
	windowedLeft := make([]float64, n)
	windowedRight := make([]float64, n)
	for i := range left {
		window := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
		windowedLeft[i] = left[i] * window
		windowedRight[i] = right[i] * window
	}
	return windowedLeft, windowedRight
}

func toComplex(values []float64) []complex128 {
	result := make([]complex128, len(values))
	for i, value := range values {
		result[i] = complex(value, 0)
	}
	return result
}

func fft(input []complex128) []complex128 {
	n := len(input)
	result := append([]complex128(nil), input...)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			result[i], result[j] = result[j], result[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		half := length / 2
		step := -2 * math.Pi / float64(length)
		for start := 0; start < n; start += length {
			for k := 0; k < half; k++ {
				twiddle := cmplx.Rect(1, step*float64(k)) * result[start+k+half]
				result[start+k+half] = result[start+k] - twiddle
				result[start+k] += twiddle
			}
		}
	}
	return result
}

func measureBand(left, right []complex128, coherenceByBand map[string]float64, band bandDef, sampleRate int) BandMetrics {
	binHz := float64(sampleRate) / float64(len(left))
	lowBin := int(math.Floor(band.low / binHz))
	highBin := int(math.Ceil(band.high / binHz))
	if lowBin < 1 {
		lowBin = 1
	}
	if highBin >= len(left)/2 {
		highBin = len(left)/2 - 1
	}

	var power, cancellationSum, balanceSum, phaseX, phaseY float64
	for bin := lowBin; bin <= highBin; bin++ {
		l := left[bin]
		r := right[bin]
		lMag := cmplx.Abs(l)
		rMag := cmplx.Abs(r)
		magSum := lMag + rMag
		if magSum <= math.Pow(10, noiseFloorDBFS/20) {
			continue
		}

		cross := l * cmplx.Conj(r)
		binPower := (lMag*lMag + rMag*rMag) / 2
		phase := cmplx.Phase(cross)
		monoMag := cmplx.Abs(l + r)
		cancellation := 1 - monoMag/magSum
		balance := (lMag - rMag) / magSum

		power += binPower
		cancellationSum += cancellation * binPower
		balanceSum += balance * binPower
		phaseX += math.Cos(phase) * binPower
		phaseY += math.Sin(phase) * binPower
	}

	metrics := BandMetrics{
		Name:      band.name,
		LowHz:     band.low,
		HighHz:    band.high,
		VocalBand: band.vocalBand,
	}
	if power > 0 {
		metrics.Coherence = coherenceByBand[band.name]
		metrics.Cancellation = cancellationSum / power
		metrics.MagnitudeBalance = balanceSum / power
		metrics.WeightedPhase = math.Atan2(phaseY/power, phaseX/power)
	}
	return metrics
}

func frequencyBands() []bandDef {
	return []bandDef{
		{name: "20-60", low: 20, high: 60},
		{name: "60-125", low: 60, high: 125},
		{name: "125-250", low: 125, high: 250},
		{name: "250-500", low: 250, high: 500, vocalBand: true},
		{name: "500-1k", low: 500, high: 1000, vocalBand: true},
		{name: "1k-2k", low: 1000, high: 2000, vocalBand: true},
		{name: "2k-4k", low: 2000, high: 4000, vocalBand: true},
		{name: "4k-8k", low: 4000, high: 8000},
		{name: "8k-16k", low: 8000, high: 16000},
	}
}

func estimateCoherence(left, right []float64, sampleRate int) map[string]float64 {
	blockCount := 4
	blockSize := nextPowerOfTwo(len(left) / blockCount)
	if blockSize < 256 {
		blockSize = 256
	}
	crossPower := make([]complex128, blockSize)
	leftPower := make([]float64, blockSize)
	rightPower := make([]float64, blockSize)
	usableBlocks := 0

	for block := 0; block < blockCount; block++ {
		start := block * len(left) / blockCount
		end := (block + 1) * len(left) / blockCount
		if end-start < 64 {
			continue
		}
		leftBlock := make([]float64, blockSize)
		rightBlock := make([]float64, blockSize)
		for i := start; i < end; i++ {
			relative := i - start
			window := 0.5 * (1 - math.Cos(2*math.Pi*float64(relative)/float64(end-start-1)))
			leftBlock[relative] = left[i] * window
			rightBlock[relative] = right[i] * window
		}
		leftSpectrum := fft(toComplex(leftBlock))
		rightSpectrum := fft(toComplex(rightBlock))
		for bin := range crossPower {
			crossPower[bin] += leftSpectrum[bin] * cmplx.Conj(rightSpectrum[bin])
			leftPower[bin] += cmplx.Abs(leftSpectrum[bin]) * cmplx.Abs(leftSpectrum[bin])
			rightPower[bin] += cmplx.Abs(rightSpectrum[bin]) * cmplx.Abs(rightSpectrum[bin])
		}
		usableBlocks++
	}

	result := make(map[string]float64)
	if usableBlocks == 0 {
		return result
	}
	binHz := float64(sampleRate) / float64(blockSize)
	for _, band := range frequencyBands() {
		lowBin := maxInt(1, int(math.Floor(band.low/binHz)))
		highBin := minInt(blockSize/2-1, int(math.Ceil(band.high/binHz)))
		var numerator, denominator float64
		for bin := lowBin; bin <= highBin; bin++ {
			numerator += cmplx.Abs(crossPower[bin])
			denominator += math.Sqrt(leftPower[bin] * rightPower[bin])
		}
		if denominator > 0 {
			result[band.name] = clamp(numerator/denominator, 0, 1)
		}
	}
	return result
}

func nextPowerOfTwo(value int) int {
	result := 1
	for result < value {
		result <<= 1
	}
	return result
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

func pearson(left, right []float64) float64 {
	var leftSum, rightSum float64
	for i := range left {
		leftSum += left[i]
		rightSum += right[i]
	}
	leftMean := leftSum / float64(len(left))
	rightMean := rightSum / float64(len(right))

	var covariance, leftVariance, rightVariance float64
	for i := range left {
		l := left[i] - leftMean
		r := right[i] - rightMean
		covariance += l * r
		leftVariance += l * l
		rightVariance += r * r
	}
	denominator := math.Sqrt(leftVariance * rightVariance)
	if denominator == 0 {
		return 0
	}
	return covariance / denominator
}

func rmsDB(samples []float64) float64 {
	var sum float64
	for _, sample := range samples {
		sum += sample * sample
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	if rms <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(rms)
}

func sumSignals(left, right []float64, gain float64) []float64 {
	result := make([]float64, len(left))
	for i := range left {
		result[i] = (left[i] + right[i]) * gain
	}
	return result
}

func differenceSignal(left, right []float64) []float64 {
	result := make([]float64, len(left))
	for i := range left {
		result[i] = left[i] - right[i]
	}
	return result
}

func wrapPhase(phase float64) float64 {
	for phase > math.Pi {
		phase -= 2 * math.Pi
	}
	for phase < -math.Pi {
		phase += 2 * math.Pi
	}
	return phase
}
