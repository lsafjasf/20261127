package gamutcompare

import (
	"fmt"
	"math"
)

type DisplaySpec struct {
	Name  string
	Red   Point
	Green Point
	Blue  Point
	White *Point
}

type TargetSpec struct {
	Name    string
	Polygon []Point
	White   *Point
}

type Options struct {
	// CenterMode is "target_white", "display_white", or "xy_origin".
	CenterMode string
	Center     *Point
	CenterName string
}

type AreaReport struct {
	XY   float64 `json:"xy"`
	UpVP float64 `json:"u_prime_v_prime"`
}

type CoverageReport struct {
	IntersectionPolygon       []Point `json:"intersection_polygon_xy,omitempty"`
	IntersectionAreaXY        float64 `json:"intersection_area_xy"`
	IntersectionAreaUpVP      float64 `json:"intersection_area_upvp"`
	TargetAreaXY              float64 `json:"target_area_xy"`
	TargetAreaUpVP            float64 `json:"target_area_upvp"`
	DisplayAreaXY             float64 `json:"display_area_xy"`
	TargetCoverage            float64 `json:"target_coverage_ratio"`
	TargetCoveragePercent     float64 `json:"target_coverage_percent"`
	TargetCoverageUpVP        float64 `json:"target_coverage_upvp_ratio"`
	TargetCoverageUpVPPercent float64 `json:"target_coverage_upvp_percent"`
	DisplayAreaInTargetRatio  float64 `json:"display_area_in_target_ratio"`
	MissingTargetAreaXY       float64 `json:"missing_target_area_xy"`
}

type DisplayReport struct {
	Name             string           `json:"name"`
	Triangle         []Point          `json:"triangle_xy"`
	Area             AreaReport       `json:"area"`
	Coverage         CoverageReport   `json:"target_coverage"`
	White            *WhiteInfo       `json:"white_mixing,omitempty"`
	Directions       DirectionReport  `json:"uncovered_directions"`
	CommonDirections *DirectionReport `json:"common_target_white_directions,omitempty"`
	Warnings         []string         `json:"warnings,omitempty"`
}

type PairComparison struct {
	A                            string  `json:"a"`
	B                            string  `json:"b"`
	LargerArea                   string  `json:"larger_xy_area"`
	AreaDifferenceXY             float64 `json:"area_difference_xy"`
	AreaRatio                    float64 `json:"area_ratio_a_over_b"`
	AreaRelativeDifference       float64 `json:"area_relative_difference"`
	HigherTargetCoverage         string  `json:"higher_target_coverage"`
	CoverageDifferencePercentPts float64 `json:"coverage_difference_percentage_points"`
	NearlyEqualArea              bool    `json:"nearly_equal_area"`
	AreaAndCoverageOrdersDiffer  bool    `json:"area_and_coverage_orders_differ"`
}

type Comparison struct {
	AreaLeaders                  []string         `json:"area_leaders"`
	UpVPAreaLeaders              []string         `json:"upvp_area_leaders"`
	TargetCoverageLeaders        []string         `json:"target_coverage_leaders"`
	LargestAreaDisplay           string           `json:"largest_area_display"`
	LargestUpVPAreaDisplay       string           `json:"largest_upvp_area_display"`
	HighestTargetCoverageDisplay string           `json:"highest_target_coverage_display"`
	AreaLeaderIsCoverageLeader   bool             `json:"area_leader_is_coverage_leader"`
	Pairs                        []PairComparison `json:"pairs,omitempty"`
}

type Report struct {
	AssumedCoordinates string          `json:"assumed_coordinates"`
	Target             TargetSummary   `json:"target"`
	Displays           []DisplayReport `json:"displays"`
	Comparison         Comparison      `json:"comparison"`
	Caveats            []string        `json:"caveats"`
}

type TargetSummary struct {
	Name        string  `json:"name"`
	AreaXY      float64 `json:"area_xy"`
	AreaUpVP    float64 `json:"area_upvp"`
	White       *Point  `json:"white,omitempty"`
	VertexCount int     `json:"vertex_count"`
}

var Caveats = []string{
	"RGB 坐标定义二维色度三角形；白点是混合/方向中心，不是第四个色域顶点，也不应参与三角形面积。",
	"xy 与 u'v' 面积只比较色度覆盖范围，不代表三维色彩体积、峰值亮度、色调映射、色深、均匀性或可靠性。",
	"三角形旋转或偏移后，面积更大也可能与目标色域交集更小；采购适配应同时看目标覆盖率和不可覆盖方向。",
	"色域面积大小不能直接解释为画质好坏。",
}

func Analyze(displays []DisplaySpec, target TargetSpec, opts Options) (*Report, error) {
	if len(displays) == 0 {
		return nil, fmt.Errorf("at least one display is required")
	}
	if len(target.Polygon) < 3 {
		return nil, fmt.Errorf("target gamut requires at least 3 polygon points")
	}
	targetPoly := ensureCCW(target.Polygon)
	if err := ValidateChromaticities(targetPoly...); err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	targetAreaXY, targetAreaUV := gamutAreas(targetPoly)
	if targetAreaXY <= geometryEps {
		return nil, fmt.Errorf("target gamut area is zero or winding/degenerate")
	}

	report := &Report{
		AssumedCoordinates: "默认按 CIE 1931 2° xy；u'v' 为 CIE 1976 UCS 换算面积",
		Target: TargetSummary{
			Name:        orDefault(target.Name, "target"),
			AreaXY:      targetAreaXY,
			AreaUpVP:    targetAreaUV,
			White:       target.White,
			VertexCount: len(targetPoly),
		},
		Caveats: append([]string(nil), Caveats...),
	}

	for _, display := range displays {
		dr, err := analyzeDisplay(display, target, targetPoly, opts)
		if err != nil {
			return nil, err
		}
		report.Displays = append(report.Displays, dr)
	}
	report.Comparison = compareDisplays(report.Displays)
	return report, nil
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func ratio(numerator, denominator float64) float64 {
	if math.Abs(denominator) < geometryEps {
		return 0
	}
	return numerator / denominator
}
