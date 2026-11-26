package gamutcompare

import (
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.12f, want %.12f ± %.2e", name, got, want, tol)
	}
}

func TestPolygonAreaKnown(t *testing.T) {
	triangle := []Point{{0, 0}, {1, 0}, {0, 1}}
	approx(t, "ccw area", PolygonArea(triangle), 0.5, 1e-12)

	reversed := []Point{{0, 0}, {0, 1}, {1, 0}}
	approx(t, "cw area", PolygonArea(reversed), 0.5, 1e-12)

	if got := PolygonArea([]Point{{0, 0}, {1, 1}}); got != 0 {
		t.Fatalf("degenerate area = %v, want 0", got)
	}
}

func TestIntersectionIdenticalTriangle(t *testing.T) {
	triangle := []Point{{0, 0}, {1, 0}, {0, 1}}
	got := IntersectionArea(triangle, triangle)
	approx(t, "identical intersection", got, 0.5, 1e-10)
}

func TestIntersectionEqualHeightDifferentShape(t *testing.T) {
	target := []Point{{0, 0}, {1, 0}, {0, 1}}
	equalHeight := []Point{{0, 0}, {1, 0}, {0.5, 1}}

	approx(t, "target area", PolygonArea(target), 0.5, 1e-12)
	approx(t, "equal-height area", PolygonArea(equalHeight), 0.5, 1e-12)

	intersection := IntersectionArea(target, equalHeight)
	approx(t, "intersection", intersection, 1.0/3.0, 1e-9)

	coverage := intersection / PolygonArea(target)
	approx(t, "target coverage", coverage, 2.0/3.0, 1e-9)
}

func TestIntersectionTranslatedEqualArea(t *testing.T) {
	target := []Point{{0, 0}, {1, 0}, {0, 1}}
	shifted := []Point{{0.2, -0.2}, {1.2, -0.2}, {0.2, 0.8}}

	approx(t, "shifted display area", PolygonArea(shifted), 0.5, 1e-12)
	approx(t, "shifted intersection", IntersectionArea(target, shifted), 0.32, 1e-9)
}

func TestIntersectionIsOrientationIndependent(t *testing.T) {
	subject := []Point{{0, 0}, {1, 0}, {0, 1}}
	clipCCW := []Point{{0, 0}, {1, 0}, {0, 1}}
	clipCW := []Point{{0, 0}, {0, 1}, {1, 0}}

	ccw := IntersectionArea(subject, clipCCW)
	cw := IntersectionArea(subject, clipCW)
	approx(t, "cw clipping result", cw, ccw, 1e-12)
}
