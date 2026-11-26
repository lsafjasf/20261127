package gamutcompare

import (
	"fmt"
	"math"
)

func analyzeDisplay(display DisplaySpec, target TargetSpec, targetPoly []Point, opts Options) (DisplayReport, error) {
	if display.Name == "" {
		display.Name = "display"
	}
	points := []Point{display.Red, display.Green, display.Blue}
	if err := ValidateChromaticities(points...); err != nil {
		return DisplayReport{}, fmt.Errorf("display %q: %w", display.Name, err)
	}
	tri := canonicalTriangle(display.Red, display.Green, display.Blue)
	areaXY, areaUV := gamutAreas(tri)
	if areaXY <= geometryEps {
		return DisplayReport{}, fmt.Errorf("display %q RGB triangle is degenerate", display.Name)
	}

	warnings := physicalWarnings(display)
	report := DisplayReport{
		Name:     display.Name,
		Triangle: tri,
		Area:     AreaReport{XY: areaXY, UpVP: areaUV},
		Warnings: warnings,
	}

	if display.White != nil && finitePoint(*display.White) {
		whiteInfo, err := PrimaryLuminanceShares(display.Red, display.Green, display.Blue, *display.White)
		if err != nil {
			report.Warnings = append(report.Warnings, err.Error())
		} else {
			report.White = &whiteInfo
			if !whiteInfo.Inside {
				report.Warnings = append(report.Warnings,
					"white lies outside or on an infeasible side of the RGB triangle; nonnegative additive mixing cannot reproduce it exactly")
			}
		}
	}

	intersection := IntersectPolygons(targetPoly, tri)
	intersectionXY := PolygonArea(intersection)
	intersectionUV := PolygonArea(uvPolygon(intersection))
	targetAreaXY := PolygonArea(targetPoly)
	_, targetAreaUV := gamutAreas(targetPoly)
	targetCoverage := clamp01(ratio(intersectionXY, targetAreaXY))
	targetCoverageUV := clamp01(ratio(intersectionUV, targetAreaUV))
	report.Coverage = CoverageReport{
		IntersectionPolygon:       intersection,
		IntersectionAreaXY:        intersectionXY,
		IntersectionAreaUpVP:      intersectionUV,
		TargetAreaXY:              targetAreaXY,
		TargetAreaUpVP:            targetAreaUV,
		DisplayAreaXY:             areaXY,
		TargetCoverage:            targetCoverage,
		TargetCoveragePercent:     targetCoverage * 100,
		TargetCoverageUpVP:        targetCoverageUV,
		TargetCoverageUpVPPercent: targetCoverageUV * 100,
		DisplayAreaInTargetRatio:  clamp01(ratio(intersectionXY, areaXY)),
		MissingTargetAreaXY:       math.Max(0, targetAreaXY-intersectionXY),
	}

	center, kind := directionCenter(display, target, tri, opts, &report)
	report.Directions = AnalyzeDirections(targetPoly, tri, center, kind)
	if target.White != nil && kind != "target_white" {
		common := AnalyzeDirections(targetPoly, tri, *target.White, "target_white (common comparison center)")
		report.CommonDirections = &common
	}
	return report, nil
}

func physicalWarnings(display DisplaySpec) []string {
	var warnings []string
	if warning := xyPhysicalWarning("red", display.Red); warning != "" {
		warnings = append(warnings, warning)
	}
	if warning := xyPhysicalWarning("green", display.Green); warning != "" {
		warnings = append(warnings, warning)
	}
	if warning := xyPhysicalWarning("blue", display.Blue); warning != "" {
		warnings = append(warnings, warning)
	}
	if display.White != nil {
		if warning := xyPhysicalWarning("white", *display.White); warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func directionCenter(display DisplaySpec, target TargetSpec, tri []Point, opts Options, report *DisplayReport) (Point, string) {
	if opts.Center != nil {
		name := opts.CenterName
		if name == "" {
			name = "custom"
		}
		return *opts.Center, name
	}

	mode := opts.CenterMode
	if mode == "" {
		mode = "display_white"
	}
	switch mode {
	case "xy_origin":
		return Point{}, "xy_origin"
	case "target_white":
		if target.White != nil {
			return *target.White, "target_white"
		}
		report.Warnings = append(report.Warnings, "target white missing; using target vertex centroid for directions")
		return vertexCentroid(target.Polygon), "target_centroid_fallback"
	case "display_white":
		if display.White != nil {
			return *display.White, "display_white"
		}
		if target.White != nil {
			report.Warnings = append(report.Warnings, "display white missing; using target white for directions")
			return *target.White, "target_white_fallback"
		}
		report.Warnings = append(report.Warnings, "display white missing; using RGB triangle centroid for directions")
		return vertexCentroid(tri), "display_centroid_fallback"
	default:
		report.Warnings = append(report.Warnings, fmt.Sprintf("unknown direction_center %q; using display_white", mode))
		if display.White != nil {
			return *display.White, "display_white"
		}
		return vertexCentroid(tri), "display_centroid_fallback"
	}
}

func vertexCentroid(poly []Point) Point {
	var sum Point
	for _, p := range poly {
		sum = add(sum, p)
	}
	if len(poly) == 0 {
		return Point{}
	}
	return scale(sum, 1/float64(len(poly)))
}

func uvPolygon(poly []Point) []Point {
	out := make([]Point, len(poly))
	for i, p := range poly {
		out[i] = p.upVPPoint()
	}
	return out
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(0, math.Min(1, v))
}
