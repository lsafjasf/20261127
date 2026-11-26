package gamutcompare

import (
	"math"
	"sort"
)

// AnalyzeDirections finds target-boundary arcs outside cover and converts them
// to angular sectors around center. Angles are mathematical xy-plane angles:
// 0 degrees is +x, increasing counter-clockwise.
func AnalyzeDirections(target, cover []Point, center Point, centerKind string) DirectionReport {
	targetCCW := ensureCCW(target)
	coverCCW := ensureCCW(cover)

	raw := outsideAngularIntervals(targetCCW, coverCCW, center)
	intervals := unionCircularIntervals(raw)

	report := DirectionReport{
		CoordinateConvention: "CIE xy plane; 0 deg = +x, counter-clockwise",
		Center:               center,
		CenterKind:           centerKind,
		CenterInsideTarget:   insideConvex(center, targetCCW),
		CenterInsideCover:    insideConvex(center, coverCCW),
		FullTargetCovered:    len(intervals) == 0,
		Sectors:              make([]MissingSector, 0, len(intervals)),
	}

	for _, interval := range intervals {
		midUnwrapped := (interval.lo + interval.hi) / 2
		midAngle := normAngle(midUnwrapped)
		midGap, targetHit, coverHit, targetOK, coverOK := radialGapAt(center, midAngle, targetCCW, coverCCW)

		sector := MissingSector{
			StartAngleDeg:       angleDeg(interval.lo),
			EndAngleDeg:         interval.hi * 180 / math.Pi,
			WidthDeg:            (interval.hi - interval.lo) * 180 / math.Pi,
			MidpointAngleDeg:    angleDeg(midAngle),
			RadialGapAtMidpoint: midGap,
			MaxRadialGap:        midGap,
			MaxGapAngleDeg:      angleDeg(midAngle),
		}
		if targetOK {
			sector.TargetRadius = targetHit.T
			sector.TargetBoundary = targetHit.Point
		}
		if coverOK {
			sector.CoverRadius = coverHit.T
			point := coverHit.Point
			sector.CoverBoundary = &point
		}

		maxGap, maxAngle := maximumSectorGap(center, interval, targetCCW, coverCCW)
		if maxGap > sector.MaxRadialGap {
			sector.MaxRadialGap = maxGap
			sector.MaxGapAngleDeg = angleDeg(maxAngle)
		}
		report.Sectors = append(report.Sectors, sector)
	}

	sort.Slice(report.Sectors, func(i, j int) bool {
		return report.Sectors[i].StartAngleDeg < report.Sectors[j].StartAngleDeg
	})
	return report
}

func ensureCCW(poly []Point) []Point {
	out := append([]Point(nil), poly...)
	if signedArea(out) < 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

func radialGapAt(center Point, angle float64, target, cover []Point) (gap float64, targetHit rayHit, coverHit rayHit, targetOK bool, coverOK bool) {
	targetHit, targetOK = rayFarIntersection(center, angle, target)
	coverHit, coverOK = rayFarIntersection(center, angle, cover)
	if !targetOK {
		return 0, targetHit, coverHit, false, coverOK
	}
	if !coverOK {
		return targetHit.T, targetHit, coverHit, true, false
	}
	return math.Max(0, targetHit.T-coverHit.T), targetHit, coverHit, true, true
}

func maximumSectorGap(center Point, interval angularInterval, target, cover []Point) (float64, float64) {
	candidates := []float64{interval.lo, interval.hi, (interval.lo + interval.hi) / 2}
	const samples = 72
	for i := 1; i < samples; i++ {
		candidates = append(candidates, interval.lo+(interval.hi-interval.lo)*float64(i)/samples)
	}
	for _, poly := range [][]Point{target, cover} {
		for _, point := range poly {
			angle := normAngle(math.Atan2(point.Y-center.Y, point.X-center.X))
			for _, candidate := range []float64{angle, angle + 2*math.Pi} {
				if candidate >= interval.lo-angleMergeEps && candidate <= interval.hi+angleMergeEps {
					candidates = append(candidates, candidate)
				}
			}
		}
	}

	bestGap := -1.0
	bestAngle := (interval.lo + interval.hi) / 2
	for _, angle := range candidates {
		gap, _, _, targetOK, _ := radialGapAt(center, angle, target, cover)
		if targetOK && gap > bestGap {
			bestGap = gap
			bestAngle = angle
		}
	}
	if bestGap < 0 {
		return 0, bestAngle
	}
	return bestGap, bestAngle
}
