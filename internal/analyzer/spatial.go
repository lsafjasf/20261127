package analyzer

import (
	"math"
	"sort"
)

func analyzeSpatial(p *plane, scale float64) SpatialStats {
	if p.W < 3 || p.H < 3 {
		return SpatialStats{}
	}
	if scale <= 0 {
		scale = 1
	}

	area := p.W * p.H
	stride := max(1, int(math.Ceil(math.Sqrt(float64(area)/1_500_000))))
	mean, variance := planeMeanVariance(p)
	_ = mean
	std := math.Sqrt(max(0, variance))

	mag := make([]float32, area)
	sampled := make([]float32, 0, area/(stride*stride)+1)
	var lapSumSq float64
	var lapCount int
	for y := 1; y < p.H-1; y++ {
		for x := 1; x < p.W-1; x++ {
			gx := (p.at(x+1, y) - p.at(x-1, y)) * 0.5
			gy := (p.at(x, y+1) - p.at(x, y-1)) * 0.5
			m := float32(math.Hypot(gx, gy))
			mag[y*p.W+x] = m
			if x%stride == 0 && y%stride == 0 {
				sampled = append(sampled, m)
				lap := p.at(x+1, y) + p.at(x-1, y) + p.at(x, y+1) + p.at(x, y-1) - 4*p.at(x, y)
				lapSumSq += lap * lap
				lapCount++
			}
		}
	}
	if len(sampled) == 0 {
		return SpatialStats{}
	}
	sort.Slice(sampled, func(i, j int) bool { return sampled[i] < sampled[j] })
	rawGradient := GradientStats{
		Mean: meanFloat32(sampled),
		RMS:  rmsFloat32(sampled),
		P50:  quantileFloat32(sampled, 0.50),
		P90:  quantileFloat32(sampled, 0.90),
		P99:  quantileFloat32(sampled, 0.99),
	}
	gradient := GradientStats{
		Mean: rawGradient.Mean * scale,
		RMS:  rawGradient.RMS * scale,
		P50:  rawGradient.P50 * scale,
		P90:  rawGradient.P90 * scale,
		P99:  rawGradient.P99 * scale,
	}

	lapRMS := 0.0
	if lapCount > 0 {
		lapRMS = math.Sqrt(lapSumSq / float64(lapCount))
	}
	lapPhysical := lapRMS * scale * scale
	hfNorm := lapPhysical / max(std, 1)

	widths := estimateEdgeWidths(p, mag, rawGradient, scale)
	sort.Float64s(widths)
	return SpatialStats{
		Gradient:        gradient,
		LaplacianRMS:    lapPhysical,
		HFNorm:          hfNorm,
		EdgeWidthMedian: quantileFloat64(widths, 0.50),
		EdgeWidthP75:    quantileFloat64(widths, 0.75),
		EdgeSamples:     len(widths),
	}
}

func planeMeanVariance(p *plane) (float64, float64) {
	var sum, sumSq float64
	for _, v := range p.P {
		x := float64(v)
		sum += x
		sumSq += x * x
	}
	n := float64(len(p.P))
	mean := sum / n
	return mean, sumSq/n - mean*mean
}

func estimateEdgeWidths(p *plane, mag []float32, gradient GradientStats, scale float64) []float64 {
	threshold := max(8, gradient.P99/scale*0.75)
	radius := 3.0 * scale
	step := max(0.15, 0.18*scale)
	rPixels := int(math.Ceil(radius + 2))
	if p.W <= 2*rPixels+2 || p.H <= 2*rPixels+2 {
		return nil
	}

	area := p.W * p.H
	stride := max(1, int(math.Ceil(math.Sqrt(float64(area)/700_000))))
	widths := make([]float64, 0, area/16)
	for y := rPixels; y < p.H-rPixels; y += stride {
		for x := rPixels; x < p.W-rPixels; x += stride {
			m := float64(mag[y*p.W+x])
			if m < threshold || !isLocalGradientMax(mag, p.W, x, y, m) {
				continue
			}
			gx := (p.at(x+1, y) - p.at(x-1, y)) * 0.5
			gy := (p.at(x, y+1) - p.at(x, y-1)) * 0.5
			hyp := math.Hypot(gx, gy)
			if hyp < 0.01 {
				continue
			}
			width, ok := edgeWidthAt(p, float64(x), float64(y), gx/hyp, gy/hyp, radius, step)
			if ok {
				widths = append(widths, width/scale)
			}
		}
	}
	return widths
}

func isLocalGradientMax(mag []float32, stride, x, y int, value float64) bool {
	center := y*stride + x
	if float64(mag[center-1]) > value || float64(mag[center+1]) > value {
		return false
	}
	if float64(mag[center-stride]) > value || float64(mag[center+stride]) > value {
		return false
	}
	return true
}

func edgeWidthAt(p *plane, x, y, dx, dy, radius, step float64) (float64, bool) {
	const sampleLimit = 96
	n := int(math.Ceil(2*radius/step)) + 1
	n = min(n, sampleLimit)
	ts := make([]float64, n)
	vs := make([]float64, n)
	for i := 0; i < n; i++ {
		t := -radius + 2*radius*float64(i)/float64(n-1)
		ts[i] = t
		vs[i] = sampleBilinear(p, x+dx*t, y+dy*t)
	}
	outer := max(2, n/6)
	low := meanSlice(vs[:outer])
	high := meanSlice(vs[n-outer:])
	contrast := high - low
	if math.Abs(contrast) < 10 {
		return 0, false
	}
	for i := range vs {
		vs[i] = (vs[i] - low) / contrast
	}

	var i10, i90 int
	level10, level90 := 0.1, 0.9
	if vs[0] <= 0.5 {
		i10 = firstCrossing(vs, level10, 1)
		if i10 >= 0 {
			i90 = firstCrossing(vs[i10:], level90, 1)
			if i90 >= 0 {
				i90 += i10
			}
		}
	} else {
		level10, level90 = 0.9, 0.1
		i10 = firstCrossing(vs, level10, -1)
		if i10 >= 0 {
			i90 = firstCrossing(vs[i10:], level90, -1)
			if i90 >= 0 {
				i90 += i10
			}
		}
	}
	if i10 < 0 || i90 < 0 {
		return 0, false
	}
	increasing := vs[0] <= 0.5
	t10 := interpolateCrossing(ts, vs, i10, level10, increasing)
	t90 := interpolateCrossing(ts, vs, i90, level90, increasing)
	width := math.Abs(t90 - t10)
	if width < 0.05 || width > 10 {
		return 0, false
	}
	return width, true
}

func firstCrossing(values []float64, level, direction float64) int {
	for i, v := range values {
		if direction > 0 && v >= level {
			return i
		}
		if direction < 0 && v <= level {
			return i
		}
	}
	return -1
}

func interpolateCrossing(ts, vs []float64, i int, level float64, increasing bool) float64 {
	if i <= 0 {
		return ts[i]
	}
	a, b := vs[i-1], vs[i]
	if math.Abs(b-a) < 1e-9 {
		return ts[i]
	}
	frac := (level - a) / (b - a)
	if !increasing {
		frac = (a - level) / (a - b)
	}
	frac = math.Max(0, math.Min(1, frac))
	return ts[i-1] + frac*(ts[i]-ts[i-1])
}

func sampleBilinear(p *plane, x, y float64) float64 {
	x = math.Max(0, math.Min(float64(p.W-1), x))
	y = math.Max(0, math.Min(float64(p.H-1), y))
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := min(x0+1, p.W-1), min(y0+1, p.H-1)
	fx, fy := x-float64(x0), y-float64(y0)
	a := p.at(x0, y0)*(1-fx) + p.at(x1, y0)*fx
	b := p.at(x0, y1)*(1-fx) + p.at(x1, y1)*fx
	return a*(1-fy) + b*fy
}

func quantileFloat32(values []float32, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return float64(values[int(q*float64(len(values)-1))])
}

func quantileFloat64(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[int(q*float64(len(values)-1))]
}

func meanFloat32(values []float32) float64 {
	var sum float64
	for _, v := range values {
		sum += float64(v)
	}
	return sum / float64(len(values))
}

func rmsFloat32(values []float32) float64 {
	var sum float64
	for _, v := range values {
		x := float64(v)
		sum += x * x
	}
	return math.Sqrt(sum / float64(len(values)))
}

func meanSlice(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
