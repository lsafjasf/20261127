package gamutcompare

import "math"

const geometryEps = 1e-10

func cross(a, b Point) float64 { return a.X*b.Y - a.Y*b.X }

func sub(a, b Point) Point { return Point{X: a.X - b.X, Y: a.Y - b.Y} }

func add(a, b Point) Point { return Point{X: a.X + b.X, Y: a.Y + b.Y} }

func scale(a Point, k float64) Point { return Point{X: a.X * k, Y: a.Y * k} }

func dot(a, b Point) float64 { return a.X*b.X + a.Y*b.Y }

func signedArea(poly []Point) float64 {
	var sum float64
	for i := range poly {
		j := (i + 1) % len(poly)
		sum += poly[i].X*poly[j].Y - poly[i].Y*poly[j].X
	}
	return sum / 2
}

// PolygonArea returns unsigned planar area in the coordinate units supplied.
func PolygonArea(poly []Point) float64 {
	if len(poly) < 3 {
		return 0
	}
	return math.Abs(signedArea(poly))
}

func canonicalTriangle(r, g, b Point) []Point {
	tri := []Point{r, g, b}
	if signedArea(tri) < 0 {
		tri[1], tri[2] = tri[2], tri[1]
	}
	return tri
}

func cleanPolygon(poly []Point) []Point {
	out := make([]Point, 0, len(poly))
	for _, p := range poly {
		if len(out) == 0 || !nearPoint(out[len(out)-1], p, geometryEps*10) {
			out = append(out, p)
		}
	}
	if len(out) > 1 && nearPoint(out[0], out[len(out)-1], geometryEps*10) {
		out = out[:len(out)-1]
	}
	if len(out) < 3 {
		return nil
	}
	return out
}

func insideConvex(p Point, poly []Point) bool {
	for i := range poly {
		a := poly[i]
		b := poly[(i+1)%len(poly)]
		if cross(sub(b, a), sub(p, a)) < -geometryEps {
			return false
		}
	}
	return true
}

func lineCrossPoint(s, e, a, b Point) Point {
	edge := sub(b, a)
	d := sub(e, s)
	den := cross(d, edge)
	if math.Abs(den) < geometryEps {
		return s
	}
	t := cross(sub(a, s), edge) / den
	return add(s, scale(d, t))
}

// IntersectPolygons clips subject by a convex, counter-clockwise clip polygon.
func IntersectPolygons(subject, clip []Point) []Point {
	if len(subject) < 3 || len(clip) < 3 {
		return nil
	}
	if signedArea(clip) < 0 {
		clip = append([]Point(nil), clip...)
		for i, j := 0, len(clip)-1; i < j; i, j = i+1, j-1 {
			clip[i], clip[j] = clip[j], clip[i]
		}
	}

	out := append([]Point(nil), subject...)
	for i := range clip {
		a := clip[i]
		b := clip[(i+1)%len(clip)]
		in := out
		out = make([]Point, 0, len(in)+4)
		if len(in) == 0 {
			return nil
		}
		prev := in[len(in)-1]
		prevIn := cross(sub(b, a), sub(prev, a)) >= -geometryEps
		for _, curr := range in {
			currIn := cross(sub(b, a), sub(curr, a)) >= -geometryEps
			switch {
			case currIn:
				if !prevIn {
					out = append(out, lineCrossPoint(prev, curr, a, b))
				}
				out = append(out, curr)
			case prevIn:
				out = append(out, lineCrossPoint(prev, curr, a, b))
			}
			prev = curr
			prevIn = currIn
		}
	}
	return cleanPolygon(out)
}

// IntersectionArea returns the planar area shared by subject and convex clip.
func IntersectionArea(subject, clip []Point) float64 {
	return PolygonArea(IntersectPolygons(subject, clip))
}
