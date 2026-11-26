package gamutcompare

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestDifferentWhitePointsDoNotChangeTriangleAreaOrCoverage(t *testing.T) {
	target, err := TargetPreset("srgb")
	if err != nil {
		t.Fatal(err)
	}
	d65 := Point{X: 0.3127, Y: 0.3290}
	d50 := Point{X: 0.3457, Y: 0.3585}

	primaries := DisplaySpec{
		Red:   target.Polygon[0],
		Green: target.Polygon[1],
		Blue:  target.Polygon[2],
	}
	withD65 := primaries
	withD65.Name = "D65-white"
	withD65.White = &d65
	withD50 := primaries
	withD50.Name = "D50-white"
	withD50.White = &d50

	report, err := Analyze([]DisplaySpec{withD65, withD50}, target, Options{CenterMode: "display_white"})
	if err != nil {
		t.Fatal(err)
	}

	a := report.Displays[0]
	b := report.Displays[1]
	approx(t, "area is invariant under white-only change", b.Area.XY, a.Area.XY, 1e-15)
	approx(t, "uv area is invariant under white-only change", b.Area.UpVP, a.Area.UpVP, 1e-12)
	approx(t, "coverage is invariant under white-only change", b.Coverage.TargetCoverage, a.Coverage.TargetCoverage, 1e-12)
	approx(t, "D65 direction center", a.Directions.Center.X, d65.X, 1e-12)
	approx(t, "D50 direction center", b.Directions.Center.X, d50.X, 1e-12)

	if a.White == nil || b.White == nil || !a.White.Inside || !b.White.Inside {
		t.Fatalf("both standard whites should mix positively: D65=%+v D50=%+v", a.White, b.White)
	}
	if math.Abs(a.White.LuminanceShares[1]-b.White.LuminanceShares[1]) < 0.01 {
		t.Fatal("different whites should produce different RGB mixing shares")
	}
	if !a.Directions.FullTargetCovered || !b.Directions.FullTargetCovered {
		t.Fatal("identical sRGB triangles should fully cover sRGB regardless of white center")
	}
}

func TestNearEqualAreaCanHaveDifferentTargetCoverage(t *testing.T) {
	target := TargetSpec{Name: "unit-target", Polygon: []Point{{0, 0}, {1, 0}, {0, 1}}}
	aligned := DisplaySpec{
		Name: "aligned",
		Red:  Point{0, 0}, Green: Point{1, 0}, Blue: Point{0, 1},
	}
	tall := DisplaySpec{
		Name: "slightly-taller-near-equal-area",
		Red:  Point{0, 0}, Green: Point{1, 0}, Blue: Point{0.5, 1.01},
	}

	report, err := Analyze([]DisplaySpec{aligned, tall}, target, Options{})
	if err != nil {
		t.Fatal(err)
	}

	alignedReport := report.Displays[0]
	tallReport := report.Displays[1]
	if !(tallReport.Area.XY > alignedReport.Area.XY) {
		t.Fatal("tall sample should be very slightly larger in area")
	}
	approx(t, "aligned coverage", alignedReport.Coverage.TargetCoverage, 1, 1e-10)
	if tallReport.Coverage.TargetCoverage > 0.68 || tallReport.Coverage.TargetCoverage < 0.66 {
		t.Fatalf("tall coverage = %.4f, want about 0.669", tallReport.Coverage.TargetCoverage)
	}

	pair := report.Comparison.Pairs[0]
	if !pair.NearlyEqualArea {
		t.Fatalf("relative area difference %.4f%% should count as near-equal", pair.AreaRelativeDifference*100)
	}
	if pair.LargerArea != "slightly-taller-near-equal-area" {
		t.Fatalf("area leader = %q", pair.LargerArea)
	}
	if pair.HigherTargetCoverage != "aligned" {
		t.Fatalf("coverage leader = %q", pair.HigherTargetCoverage)
	}
	if !pair.AreaAndCoverageOrdersDiffer || report.Comparison.AreaLeaderIsCoverageLeader {
		t.Fatal("near-equal area sample should demonstrate area/coverage order mismatch")
	}
}

func TestSampleInputAnalysesAndRendersCaveat(t *testing.T) {
	displays, target, options, err := ParseInput(SampleInputJSON)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(displays, target, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Displays) != 2 {
		t.Fatalf("display count = %d, want 2", len(report.Displays))
	}
	for _, display := range report.Displays {
		if display.Coverage.TargetCoverage < 0.999 {
			t.Fatalf("sample %s coverage = %.4f, want full sRGB coverage", display.Name, display.Coverage.TargetCoverage)
		}
	}

	text := FormatText(report)
	if !strings.Contains(text, "色域面积大小不能直接解释为画质好坏") {
		t.Fatal("text report is missing the image-quality caveat")
	}
	if _, err := json.Marshal(report); err != nil {
		t.Fatalf("report must be JSON serializable: %v", err)
	}
}
