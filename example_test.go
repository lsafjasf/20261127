package gamutcompare

import (
	"os"
	"testing"
)

func TestEqualHeightExampleDocumentedValues(t *testing.T) {
	data, err := os.ReadFile("examples/equal_height_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	displays, target, options, err := ParseInput(data)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(displays, target, options)
	if err != nil {
		t.Fatal(err)
	}

	shifted := report.Displays[1]
	approx(t, "equal-height sample area", shifted.Area.XY, report.Displays[0].Area.XY, 1e-12)
	approx(t, "equal-height sample coverage", shifted.Coverage.TargetCoverage, 2.0/3.0, 1e-9)
	if len(shifted.Directions.Sectors) != 1 {
		t.Fatalf("sector count = %d, want 1", len(shifted.Directions.Sectors))
	}
	sector := shifted.Directions.Sectors[0]
	approx(t, "sector start", sector.StartAngleDeg, 0.1751, 0.01)
	approx(t, "sector end", sector.EndAngleDeg, 124.5673, 0.01)
	if sector.RadialGapAtMidpoint <= 0.12 || sector.RadialGapAtMidpoint >= 0.14 {
		t.Fatalf("midpoint radial gap = %.5f, want about 0.131", sector.RadialGapAtMidpoint)
	}
	if sector.MaxRadialGap < sector.RadialGapAtMidpoint {
		t.Fatal("max radial gap must be at least the midpoint gap")
	}
}
