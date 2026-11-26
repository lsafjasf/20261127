package align

import (
	"math"
	"sort"
)

type anchor struct {
	event      string
	kind       Kind
	local      float64
	master     float64
	confidence float64
	residualMS float64
}

type fitResult struct {
	slope          float64
	intercept      float64
	inliers        []anchor
	rejected       int
	baseline       float64
	rmsMS          float64
	maxMS          float64
	driftConfirmed bool
	status         string
}

type line struct {
	slope     float64
	intercept float64
	score     float64
	inliers   []anchor
}

func robustFit(input []anchor, cfg Config) *fitResult {
	anchors := make([]anchor, 0, len(input))
	for _, point := range input {
		if point.confidence+1e-12 >= cfg.MinAnchorConfidence {
			anchors = append(anchors, point)
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	sort.Slice(anchors, func(i, j int) bool {
		if anchors[i].local == anchors[j].local {
			return anchors[i].event < anchors[j].event
		}
		return anchors[i].local < anchors[j].local
	})

	if len(anchors) == 1 {
		return fitSingle(anchors[0], len(input)-1, cfg)
	}

	candidates := make([]line, 0, len(anchors)*(len(anchors)-1)/2)
	for i := 0; i < len(anchors); i++ {
		for j := i + 1; j < len(anchors); j++ {
			slope, ok := pairSlope(anchors[i], anchors[j], cfg)
			if !ok {
				continue
			}
			line := line{slope: slope, intercept: anchors[i].master - slope*anchors[i].local}
			line.inliers = inliersFor(anchors, line.slope, line.intercept, cfg.AnchorToleranceMS)
			line.score = scoreLine(line)
			candidates = append(candidates, line)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if math.Abs(candidates[i].slope-1) != math.Abs(candidates[j].slope-1) {
			return math.Abs(candidates[i].slope-1) < math.Abs(candidates[j].slope-1)
		}
		if candidates[i].slope != candidates[j].slope {
			return candidates[i].slope < candidates[j].slope
		}
		return candidates[i].intercept < candidates[j].intercept
	})

	best := candidates[0]
	if len(best.inliers) < cfg.MinOffsetAnchors {
		return nil
	}
	for round := 0; round < 3; round++ {
		slope, intercept, ok := weightedLine(best.inliers)
		if !ok || !plausibleSlope(slope, cfg) {
			return nil
		}
		next := inliersFor(anchors, slope, intercept, cfg.AnchorToleranceMS)
		if sameMembers(best.inliers, next) {
			best = line{slope: slope, intercept: intercept, inliers: next}
			break
		}
		if len(next) < cfg.MinOffsetAnchors {
			return nil
		}
		best = line{slope: slope, intercept: intercept,
			inliers: next, score: scoreLine(line{slope: slope, intercept: intercept, inliers: next})}
	}
	if len(best.inliers) < cfg.MinOffsetAnchors {
		return nil
	}
	return makeFit(anchors, best, cfg)
}

func fitSingle(point anchor, rejected int, cfg Config) *fitResult {
	status := "provisional"
	if cfg.MinOffsetAnchors <= 1 {
		status = "locked"
	}
	return &fitResult{
		slope: 1, intercept: point.master - point.local,
		inliers: []anchor{point}, rejected: rejected,
		rmsMS: 0, maxMS: 0, status: status,
	}
}

func pairSlope(a, b anchor, cfg Config) (float64, bool) {
	dt := b.local - a.local
	if math.Abs(dt) < 1e-6 {
		return 0, false
	}
	slope := (b.master - a.master) / dt
	if !plausibleSlope(slope, cfg) {
		return 0, false
	}
	return slope, true
}

func plausibleSlope(slope float64, cfg Config) bool {
	return finite(slope) && math.Abs(slope-1)*1e6 <= cfg.MaxDriftPPM
}

func inliersFor(anchors []anchor, slope, intercept float64, toleranceMS float64) []anchor {
	var result []anchor
	for _, point := range anchors {
		predicted := slope*point.local + intercept
		if math.Abs(predicted-point.master)*1000 <= toleranceMS+1e-9 {
			result = append(result, point)
		}
	}
	return result
}

func scoreLine(candidate line) float64 {
	if len(candidate.inliers) == 0 {
		return 0
	}
	support := 0.0
	events := make(map[string]bool)
	for _, point := range candidate.inliers {
		support += point.confidence
		events[point.event] = true
	}
	span := candidate.inliers[len(candidate.inliers)-1].local - candidate.inliers[0].local
	return float64(len(candidate.inliers))*1000 + float64(len(events))*10 + support + span/1e6
}

func weightedLine(points []anchor) (float64, float64, bool) {
	var sw, swl, swm, swll, swlm float64
	for _, point := range points {
		weight := math.Max(point.confidence, 1e-6)
		sw += weight
		swl += weight * point.local
		swm += weight * point.master
		swll += weight * point.local * point.local
		swlm += weight * point.local * point.master
	}
	if sw == 0 {
		return 0, 0, false
	}
	denominator := sw*swll - swl*swl
	if math.Abs(denominator) < 1e-12 {
		return 0, 0, false
	}
	slope := (sw*swlm - swl*swm) / denominator
	intercept := (swm - slope*swl) / sw
	return slope, intercept, true
}

func makeFit(anchors []anchor, best line, cfg Config) *fitResult {
	var sumsq float64
	maxMS := 0.0
	for i := range best.inliers {
		residual := (best.slope*best.inliers[i].local + best.intercept - best.inliers[i].master) * 1000
		best.inliers[i].residualMS = residual
		sumsq += residual * residual
		maxMS = math.Max(maxMS, math.Abs(residual))
	}
	rmsMS := 0.0
	if len(best.inliers) > 0 {
		rmsMS = math.Sqrt(sumsq / float64(len(best.inliers)))
	}
	baseline := best.inliers[len(best.inliers)-1].local - best.inliers[0].local
	driftConfirmed := len(best.inliers) >= cfg.MinDriftAnchors &&
		baseline >= cfg.MinDriftBaselineSec &&
		plausibleSlope(best.slope, cfg) &&
		baseline > 1e-6
	return &fitResult{
		slope: best.slope, intercept: best.intercept,
		inliers: best.inliers, rejected: len(anchors) - len(best.inliers),
		baseline: baseline, rmsMS: rmsMS, maxMS: maxMS,
		driftConfirmed: driftConfirmed, status: "locked",
	}
}

func sameMembers(a, b []anchor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].event != b[i].event || a[i].local != b[i].local {
			return false
		}
	}
	return true
}

func (fit *fitResult) mapTime(local float64) float64 {
	return fit.slope*local + fit.intercept
}
