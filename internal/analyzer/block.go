package analyzer

import (
	"math"
	"sort"
)

const flatRangeThreshold = 24.0

func analyzeBlocks(mean *plane, sizes []int) BlockStats {
	if mean.W < 8 || mean.H < 8 {
		return BlockStats{Best: BlockSizeStats{Phase: -1}}
	}
	dx := horizontalLineDiff(mean)
	dy := verticalLineDiff(mean)
	sizes = normalizeBlockSizes(sizes)
	var candidates []BlockSizeStats
	for _, size := range sizes {
		if size >= mean.W || size >= mean.H {
			continue
		}
		if r, ok := detectBlockAxis(dx, size, "horizontal"); ok {
			candidates = append(candidates, r)
		}
		if r, ok := detectBlockAxis(dy, size, "vertical"); ok {
			candidates = append(candidates, r)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Confidence != candidates[j].Confidence {
			return candidates[i].Confidence > candidates[j].Confidence
		}
		return candidates[i].Size < candidates[j].Size
	})
	result := BlockStats{Candidates: candidates}
	if len(candidates) > 0 {
		result.Best = candidates[0]
	} else {
		result.Best.Phase = -1
	}
	return result
}

func normalizeBlockSizes(sizes []int) []int {
	if len(sizes) == 0 {
		return []int{8, 16, 32}
	}
	seen := make(map[int]bool)
	var out []int
	for _, size := range sizes {
		if size >= 4 && !seen[size] {
			seen[size] = true
			out = append(out, size)
		}
	}
	sort.Ints(out)
	return out
}

func horizontalLineDiff(p *plane) []float64 {
	out := make([]float64, max(0, p.W-1))
	if len(out) == 0 {
		return out
	}
	sums := make([]float64, len(out))
	counts := make([]int, len(out))
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W-1; x++ {
			if localRange3(p, x, y, x+1, y) > flatRangeThreshold {
				continue
			}
			sums[x] += math.Abs(p.at(x+1, y) - p.at(x, y))
			counts[x]++
		}
	}
	for x := range out {
		if counts[x] < max(2, p.H/20) {
			out[x] = math.NaN()
			continue
		}
		out[x] = sums[x] / float64(counts[x])
	}
	return out
}

func verticalLineDiff(p *plane) []float64 {
	out := make([]float64, max(0, p.H-1))
	if len(out) == 0 {
		return out
	}
	sums := make([]float64, len(out))
	counts := make([]int, len(out))
	for y := 0; y < p.H-1; y++ {
		for x := 0; x < p.W; x++ {
			if localRange3(p, x, y, x, y+1) > flatRangeThreshold {
				continue
			}
			sums[y] += math.Abs(p.at(x, y+1) - p.at(x, y))
			counts[y]++
		}
	}
	for y := range out {
		if counts[y] < max(2, p.W/20) {
			out[y] = math.NaN()
			continue
		}
		out[y] = sums[y] / float64(counts[y])
	}
	return out
}

func localRange3(p *plane, ax, ay, bx, by int) float64 {
	x0 := max(0, min(ax, bx)-1)
	x1 := min(p.W-1, max(ax, bx)+1)
	y0 := max(0, min(ay, by)-1)
	y1 := min(p.H-1, max(ay, by)+1)
	low, high := math.Inf(1), math.Inf(-1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			v := p.at(x, y)
			low = math.Min(low, v)
			high = math.Max(high, v)
		}
	}
	return high - low
}

func detectBlockAxis(d []float64, size int, axis string) (BlockSizeStats, bool) {
	if len(d) < size*3 {
		return BlockSizeStats{}, false
	}
	best := BlockSizeStats{Size: size, Axis: axis, Phase: -1}
	for phase := 0; phase < size; phase++ {
		var boundarySum, interiorSum float64
		var boundaryN, interiorN int
		for i, v := range d {
			if math.IsNaN(v) {
				continue
			}
			if i%size == phase {
				boundarySum += v
				boundaryN++
			} else {
				interiorSum += v
				interiorN++
			}
		}
		if boundaryN == 0 || interiorN == 0 {
			continue
		}
		boundaryMean := boundarySum / float64(boundaryN)
		interiorMean := interiorSum / float64(interiorN)
		var interiorSq float64
		for i, v := range d {
			if math.IsNaN(v) {
				continue
			}
			if i%size != phase {
				delta := v - interiorMean
				interiorSq += delta * delta
			}
		}
		interiorStd := math.Sqrt(interiorSq / float64(interiorN))
		ratio := boundaryMean / max(interiorMean, 0.02)
		z := 0.0
		if interiorStd > 1e-9 {
			z = (boundaryMean - interiorMean) / (interiorStd / math.Sqrt(float64(boundaryN)))
		} else if boundaryMean > interiorMean+0.02 {
			z = 10
		}
		candidate := BlockSizeStats{
			Size:     size,
			Axis:     axis,
			Phase:    phase,
			Ratio:    ratio,
			Excess:   ratio - 1,
			ZScore:   z,
			Boundary: boundaryMean,
			Interior: interiorMean,
		}
		candidate.Confidence = blockConfidence(candidate)
		if betterBlockCandidate(candidate, best) {
			best = candidate
		}
	}
	return best, best.Phase >= 0
}

func blockConfidence(c BlockSizeStats) float64 {
	ratioPart := clamp((c.Ratio-1.02)/0.18, 0, 1)
	zPart := clamp((c.ZScore-1)/5, 0, 1)
	return 0.55*ratioPart + 0.45*zPart
}

func betterBlockCandidate(a, b BlockSizeStats) bool {
	if math.Abs(a.Confidence-b.Confidence) > 1e-9 {
		return a.Confidence > b.Confidence
	}
	if math.Abs(a.ZScore-b.ZScore) > 1e-9 {
		return a.ZScore > b.ZScore
	}
	return a.Size < b.Size
}
