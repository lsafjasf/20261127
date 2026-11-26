package gamutcompare

import (
	"math"
	"sort"
)

// MissingSector describes an angular range around DirectionCenter in which the
// target boundary cannot be reached by the covering gamut.
type MissingSector struct {
	StartAngleDeg       float64 `json:"start_angle_deg"`
	EndAngleDeg         float64 `json:"end_angle_deg"`
	WidthDeg            float64 `json:"width_deg"`
	MidpointAngleDeg    float64 `json:"midpoint_angle_deg"`
	TargetRadius        float64 `json:"midpoint_target_radius"`
	CoverRadius         float64 `json:"midpoint_cover_radius"`
	RadialGapAtMidpoint float64 `json:"radial_gap_at_midpoint"`
	MaxRadialGap        float64 `json:"max_radial_gap"`
	MaxGapAngleDeg      float64 `json:"max_gap_angle_deg"`
	TargetBoundary      Point   `json:"midpoint_target_boundary_point"`
	CoverBoundary       *Point  `json:"midpoint_cover_boundary_point,omitempty"`
}

// DirectionReport contains circular uncovered-direction sectors.
type DirectionReport struct {
	CoordinateConvention string          `json:"angle_convention"`
	Center               Point           `json:"center"`
	CenterKind           string          `json:"center_kind"`
	CenterInsideTarget   bool            `json:"center_inside_target"`
	CenterInsideCover    bool            `json:"center_inside_cover"`
	FullTargetCovered    bool            `json:"full_target_covered"`
	Sectors              []MissingSector `json:"sectors"`
}

type angularInterval struct{ lo, hi float64 }

func edgeBreakpoints(a, b Point, cover []Point) []float64 {
	e := sub(b, a)
	ts := []float64{0, 1}
	addT := func(t float64) {
		if t >= -geometryEps && t <= 1+geometryEps {
			ts = append(ts, math.Max(0, math.Min(1, t)))
		}
	}

	for i := range cover {
		c := cover[i]
		d := cover[(i+1)%len(cover)]
		f := sub(d, c)
		den := cross(e, f)
		q := sub(c, a)
		if math.Abs(den) > geometryEps {
			t := cross(q, f) / den
			u := cross(q, e) / den
			if t >= -geometryEps && t <= 1+geometryEps && u >= -geometryEps && u <= 1+geometryEps {
				addT(t)
			}
			continue
		}
		if math.Abs(cross(q, e)) <= geometryEps && dot(e, e) > geometryEps {
			addT(dot(sub(c, a), e) / dot(e, e))
			addT(dot(sub(d, a), e) / dot(e, e))
		}
	}

	sort.Float64s(ts)
	out := ts[:0]
	for _, t := range ts {
		t = math.Max(0, math.Min(1, t))
		if len(out) == 0 || math.Abs(t-out[len(out)-1]) > 1e-9 {
			out = append(out, t)
		}
	}
	return out
}

func outsideAngularIntervals(target, cover []Point, center Point) []angularInterval {
	var intervals []angularInterval
	for i := range target {
		a := target[i]
		b := target[(i+1)%len(target)]
		e := sub(b, a)
		ts := edgeBreakpoints(a, b, cover)
		for k := 0; k+1 < len(ts); k++ {
			t0, t1 := ts[k], ts[k+1]
			if t1-t0 <= 1e-12 {
				continue
			}
			tm := (t0 + t1) / 2
			mid := add(a, scale(e, tm))
			if insideConvex(mid, cover) {
				continue
			}
			p0 := add(a, scale(e, t0))
			p1 := add(a, scale(e, t1))
			interval, ok := segmentAngleInterval(p0, p1, center)
			if ok {
				intervals = append(intervals, interval)
			}
		}
	}
	return intervals
}

func segmentAngleInterval(p0, p1, center Point) (angularInterval, bool) {
	v0 := sub(p0, center)
	v1 := sub(p1, center)
	r0 := math.Hypot(v0.X, v0.Y)
	r1 := math.Hypot(v1.X, v1.Y)
	if r0 <= geometryEps || r1 <= geometryEps {
		return angularInterval{}, false
	}
	a0 := math.Atan2(v0.Y, v0.X)
	delta := math.Atan2(cross(v0, v1), dot(v0, v1))
	loRaw := math.Min(a0, a0+delta)
	width := math.Abs(delta)
	lo := normAngle(loRaw)
	return angularInterval{lo: lo, hi: lo + width}, true
}
