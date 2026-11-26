package analyzer

import (
	"fmt"
	"math"
	"sort"
)

func analyzeSequence(paths []string, crop Rect, gradientScale float64, blockSizes []int) (SequenceStats, error) {
	if len(paths) == 0 {
		return SequenceStats{}, fmt.Errorf("empty frame sequence")
	}
	var spatialFrames []SpatialStats
	var meanSum, temporalSum, temporalSq []float64
	var width, height int
	var previous []float32
	var totalDelta float64
	var deltaPairs int
	var globalSum, globalSq float64

	for index, path := range paths {
		full, err := decodeLuma(path)
		if err != nil {
			return SequenceStats{}, err
		}
		frame := full
		if crop.X != 0 || crop.Y != 0 || crop.W != full.W || crop.H != full.H {
			frame, err = cropPlane(full, crop)
			if err != nil {
				return SequenceStats{}, fmt.Errorf("%s: %w", path, err)
			}
		}
		if index == 0 {
			width, height = frame.W, frame.H
			n := width * height
			meanSum = make([]float64, n)
			temporalSum = make([]float64, n)
			temporalSq = make([]float64, n)
		} else if frame.W != width || frame.H != height {
			return SequenceStats{}, fmt.Errorf("%s: frame is %dx%d, expected %dx%d", path, frame.W, frame.H, width, height)
		}

		spatialFrames = append(spatialFrames, analyzeSpatial(frame, gradientScale))
		for i, v := range frame.P {
			x := float64(v)
			meanSum[i] += x
			temporalSum[i] += x
			temporalSq[i] += x * x
			globalSum += x
			globalSq += x * x
			if previous != nil {
				totalDelta += math.Abs(x - float64(previous[i]))
			}
		}
		if previous != nil {
			deltaPairs++
		}
		previous = frame.P
	}

	nFrames := float64(len(paths))
	nPixels := float64(width * height)
	meanPlane := &plane{W: width, H: height, P: make([]float32, width*height)}
	var temporalStdSum float64
	for i := range meanPlane.P {
		mean := meanSum[i] / nFrames
		meanPlane.P[i] = float32(mean)
		variance := temporalSq[i]/nFrames - mean*mean
		temporalStdSum += math.Sqrt(max(0, variance))
	}
	globalMean := globalSum / (nPixels * nFrames)
	globalStd := math.Sqrt(max(0, globalSq/(nPixels*nFrames)-globalMean*globalMean))
	meanAbsDelta := 0.0
	if deltaPairs > 0 {
		meanAbsDelta = totalDelta / (nPixels * float64(deltaPairs))
	}
	edgeNoise := weightedTemporalNoise(meanPlane, temporalSum, temporalSq, nFrames)

	return SequenceStats{
		Frames:  len(paths),
		Width:   width,
		Height:  height,
		Spatial: medianSpatial(spatialFrames),
		Block:   analyzeBlocks(meanPlane, blockSizes),
		Temporal: TemporalStats{
			Frames:        len(paths),
			MeanAbsDelta:  meanAbsDelta,
			DeltaNorm:     meanAbsDelta / max(globalStd, 1),
			EdgeNoise:     edgeNoise,
			EdgeNoiseNorm: edgeNoise / max(globalStd, 1),
		},
	}, nil
}

func medianSpatial(frames []SpatialStats) SpatialStats {
	if len(frames) == 0 {
		return SpatialStats{}
	}
	field := func(get func(SpatialStats) float64) float64 {
		values := make([]float64, len(frames))
		for i, frame := range frames {
			values[i] = get(frame)
		}
		sort.Float64s(values)
		return quantileFloat64(values, 0.5)
	}
	gradientField := func(get func(GradientStats) float64) float64 {
		values := make([]float64, len(frames))
		for i, frame := range frames {
			values[i] = get(frame.Gradient)
		}
		sort.Float64s(values)
		return quantileFloat64(values, 0.5)
	}
	return SpatialStats{
		Gradient: GradientStats{
			Mean: gradientField(func(g GradientStats) float64 { return g.Mean }),
			RMS:  gradientField(func(g GradientStats) float64 { return g.RMS }),
			P50:  gradientField(func(g GradientStats) float64 { return g.P50 }),
			P90:  gradientField(func(g GradientStats) float64 { return g.P90 }),
			P99:  gradientField(func(g GradientStats) float64 { return g.P99 }),
		},
		LaplacianRMS:    field(func(s SpatialStats) float64 { return s.LaplacianRMS }),
		HFNorm:          field(func(s SpatialStats) float64 { return s.HFNorm }),
		EdgeWidthMedian: field(func(s SpatialStats) float64 { return s.EdgeWidthMedian }),
		EdgeWidthP75:    field(func(s SpatialStats) float64 { return s.EdgeWidthP75 }),
		EdgeSamples:     int(field(func(s SpatialStats) float64 { return float64(s.EdgeSamples) })),
	}
}

func weightedTemporalNoise(mean *plane, sum, sumSq []float64, frames float64) float64 {
	if mean.W < 3 || mean.H < 3 || frames < 2 {
		return 0
	}
	mag := make([]float64, mean.W*mean.H)
	var sampled []float64
	for y := 1; y < mean.H-1; y++ {
		for x := 1; x < mean.W-1; x++ {
			gx := (mean.at(x+1, y) - mean.at(x-1, y)) * 0.5
			gy := (mean.at(x, y+1) - mean.at(x, y-1)) * 0.5
			m := math.Hypot(gx, gy)
			mag[y*mean.W+x] = m
			sampled = append(sampled, m)
		}
	}
	if len(sampled) == 0 {
		return 0
	}
	sort.Float64s(sampled)
	threshold := max(5, quantileFloat64(sampled, 0.85))
	var weighted, weightSum float64
	for i, m := range mag {
		if m < threshold {
			continue
		}
		meanValue := sum[i] / frames
		variance := max(0, sumSq[i]/frames-meanValue*meanValue)
		std := math.Sqrt(variance)
		w := m - threshold + 1
		weighted += w * std
		weightSum += w
	}
	if weightSum == 0 {
		return 0
	}
	return weighted / weightSum
}
