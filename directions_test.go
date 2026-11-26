package gamutcompare

import (
	"math"
	"testing"
)

func equilateral(radius float64) []Point {
	angles := []float64{math.Pi / 2, 7 * math.Pi / 6, 11 * math.Pi / 6}
	poly := make([]Point, 0, 3)
	for _, angle := range angles {
		poly = append(poly, Point{X: radius * math.Cos(angle), Y: radius * math.Sin(angle)})
	}
	return poly
}

func TestDirectionsIdenticalGamutHasNoMissingSector(t *testing.T) {
	triangle := []Point{{0, 0}, {1, 0}, {0, 1}}
	center := Point{X: 1.0 / 3.0, Y: 1.0 / 3.0}
	report := AnalyzeDirections(triangle, triangle, center, "test")

	if !report.FullTargetCovered || len(report.Sectors) != 0 {
		t.Fatalf("identical gamut produced missing sectors: %+v", report.Sectors)
	}
}

func TestDirectionsSmallerConcentricTriangleIsMissingEveryDirection(t *testing.T) {
	target := equilateral(1)
	cover := equilateral(0.5)
	report := AnalyzeDirections(target, cover, Point{}, "origin")

	if report.FullTargetCovered {
		t.Fatal("smaller concentric gamut should not fully cover target")
	}
	if len(report.Sectors) != 1 {
		t.Fatalf("sector count = %d, want one circular sector: %+v", len(report.Sectors), report.Sectors)
	}
	approx(t, "full-circle sector width", report.Sectors[0].WidthDeg, 360, 0.01)
	if report.Sectors[0].MaxRadialGap <= 0 {
		t.Fatalf("max radial gap = %v, want > 0", report.Sectors[0].MaxRadialGap)
	}
}

func TestDirectionsSingleClippedCornerSector(t *testing.T) {
	target := []Point{{0, 0}, {1, 0}, {0, 1}}
	cover := []Point{{0, 0}, {1, 0}, {0.2, 0.8}, {0, 0.8}}
	center := Point{X: 1.0 / 3.0, Y: 1.0 / 3.0}
	report := AnalyzeDirections(target, cover, center, "centroid")

	if len(report.Sectors) != 1 {
		t.Fatalf("sector count = %d, want 1; sectors=%+v", len(report.Sectors), report.Sectors)
	}
	sector := report.Sectors[0]
	approx(t, "clipped corner midpoint", sector.MidpointAngleDeg, 115.745, 0.05)
	if sector.WidthDeg < 18 || sector.WidthDeg > 22 {
		t.Fatalf("sector width = %.3f°, want about 19.6°", sector.WidthDeg)
	}
	cornerAngle := angleDeg(math.Atan2(2.0/3.0, -1.0/3.0))
	if cornerAngle < sector.StartAngleDeg || cornerAngle > sector.EndAngleDeg {
		t.Fatalf("corner angle %.3f not inside sector %.3f..%.3f", cornerAngle, sector.StartAngleDeg, sector.EndAngleDeg)
	}
	if sector.MaxRadialGap <= 0 {
		t.Fatalf("max radial gap = %v, want > 0", sector.MaxRadialGap)
	}
}

func TestDirectionsContainingGamutHasNoMissingSector(t *testing.T) {
	target := []Point{{0, 0}, {1, 0}, {0, 1}}
	cover := []Point{{-1, -1}, {2, -1}, {-1, 2}}
	center := Point{X: 0.2, Y: 0.2}
	report := AnalyzeDirections(target, cover, center, "inside")

	if !report.FullTargetCovered {
		t.Fatalf("containing gamut reported sectors: %+v", report.Sectors)
	}
	if !report.CenterInsideCover || !report.CenterInsideTarget {
		t.Fatal("test center should be inside both polygons")
	}
}
