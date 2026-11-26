package gamutcompare

import "math"

type rayHit struct {
	T     float64
	Point Point
	U     float64
}

func raySegment(center, dir, a, b Point) (rayHit, bool) {
	edge := sub(b, a)
	den := cross(dir, edge)
	if math.Abs(den) < geometryEps {
		return rayHit{}, false
	}
	q := sub(a, center)
	t := cross(q, edge) / den
	u := cross(q, dir) / den
	if t < -geometryEps || u < -geometryEps || u > 1+geometryEps {
		return rayHit{}, false
	}
	if t < 0 {
		t = 0
	}
	return rayHit{T: t, Point: add(center, scale(dir, t)), U: u}, true
}

func rayFarIntersection(center Point, angle float64, poly []Point) (rayHit, bool) {
	dir := Point{X: math.Cos(angle), Y: math.Sin(angle)}
	var farthest rayHit
	found := false
	for i := range poly {
		a := poly[i]
		b := poly[(i+1)%len(poly)]
		hit, ok := raySegment(center, dir, a, b)
		if !ok {
			continue
		}
		if !found || hit.T > farthest.T {
			farthest = hit
			found = true
		}
	}
	return farthest, found
}

func normAngle(a float64) float64 {
	twoPi := 2 * math.Pi
	a = math.Mod(a, twoPi)
	if a < 0 {
		a += twoPi
	}
	return a
}

func angleDeg(a float64) float64 { return normAngle(a) * 180 / math.Pi }
