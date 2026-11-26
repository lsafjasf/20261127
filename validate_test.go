package rgbxyz

import (
	"reflect"
	"testing"
)

func TestHealthyMatrixPassesAllChecks(t *testing.T) {
	report := Diagnose(SRGBToXYZD65())
	if !report.OK {
		t.Fatalf("healthy matrix failed: %v", report.Issues)
	}
	if !report.Invertible || report.Handedness != HandednessRight {
		t.Fatalf("invertible=%t handedness=%s", report.Invertible, report.Handedness)
	}
	if !report.WhitePoint.OK {
		t.Fatalf("white point error = %g", report.WhitePoint.MaxError)
	}
	if len(report.Probes) != 3 {
		t.Fatalf("got %d probes, want 3", len(report.Probes))
	}
	for _, probe := range report.Probes {
		if probe.Polarity != PolarityAligned {
			t.Errorf("axis %s polarity = %s, want aligned", probe.AxisName, probe.Polarity)
		}
		if probe.AntisymmetryErr > 1e-15 {
			t.Errorf("axis %s antisymmetry error = %g", probe.AxisName, probe.AntisymmetryErr)
		}
		if MaxDifference(probe.PositiveOutput, probe.NegativeOutput.Scale(-1)) > 1e-15 {
			t.Errorf("axis %s did not map +e and -e antisymmetrically", probe.AxisName)
		}
	}
}

func TestReflectedBlueColumnIsLocated(t *testing.T) {
	faulty, err := FlipColumn(SRGBToXYZD65(), 2)
	if err != nil {
		t.Fatal(err)
	}
	report := Diagnose(faulty)
	if !report.Invertible {
		t.Fatal("reflected matrix should still be invertible")
	}
	if report.Handedness != HandednessLeft {
		t.Fatalf("handedness = %s, want left-handed", report.Handedness)
	}
	if report.WhitePoint.OK {
		t.Fatal("reflected matrix unexpectedly passed the white-point check")
	}
	if !reflect.DeepEqual(report.FlippedAxes, []int{2}) {
		t.Fatalf("flipped axes = %v, want [2]", report.FlippedAxes)
	}
	if report.Probes[0].Polarity != PolarityAligned ||
		report.Probes[1].Polarity != PolarityAligned {
		t.Fatal("red or green axis was incorrectly reported as flipped")
	}
	blue := report.Probes[2]
	if blue.Polarity != PolarityFlipped {
		t.Fatalf("blue polarity = %s, want flipped", blue.Polarity)
	}
	if MaxDifference(blue.PositiveInput, (Vec3{0, 0, 1})) != 0 ||
		MaxDifference(blue.NegativeInput, (Vec3{0, 0, -1})) != 0 {
		t.Fatalf("blue probe inputs = %v / %v", blue.PositiveInput, blue.NegativeInput)
	}
}

func TestRepairRecoversOriginalAndPreservesError(t *testing.T) {
	reference := SRGBToXYZD65()
	original := Mat3{
		{0.41246, 0.35758, 0.18044},
		{0.21267, 0.71516, 0.07218},
		{0.01933, 0.11919, 0.95030},
	}
	faulty, err := FlipColumn(original, 2)
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.Reference = &original

	repaired, report, err := Repair(faulty, config)
	if err != nil {
		t.Fatalf("Repair() error = %v", err)
	}
	if !report.OK {
		t.Fatalf("repaired matrix failed: %v", report.Issues)
	}
	if MaxMatrixDifference(original, repaired) != 0 {
		t.Fatalf("repaired matrix differs from original by %g",
			MaxMatrixDifference(original, repaired))
	}

	before := CompareColors(reference, original, SampleColors(), D65White())
	after := CompareColors(reference, repaired, SampleColors(), D65White())
	if before.MaxDeltaE == 0 {
		t.Fatal("test fixture must have a non-zero original color error")
	}
	if !ErrorStatsEqual(before, after, 1e-12) {
		t.Fatalf("color error changed: before=%+v after=%+v", before, after)
	}
	if MaxXYZ := CompareColors(original, repaired, SampleColors(), D65White()).MaxXYZ; MaxXYZ != 0 {
		t.Fatalf("repaired-vs-original max XYZ error = %g", MaxXYZ)
	}
}

func TestWhitePointFailureWithPositiveDeterminant(t *testing.T) {
	matrix := SRGBToXYZD65()
	matrix[1][0] *= 1.1
	report := Diagnose(matrix)
	if !report.Invertible || report.Handedness != HandednessRight {
		t.Fatalf("test matrix should remain invertible and right-handed")
	}
	if report.WhitePoint.OK {
		t.Fatal("white point should fail")
	}
	if len(report.FlippedAxes) != 0 {
		t.Fatalf("unexpected axis flips: %v", report.FlippedAxes)
	}
}

func TestRepairWithoutReferenceUsesWhitePoint(t *testing.T) {
	original := SRGBToXYZD65()
	faulty, err := FlipColumn(original, 1)
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.Reference = nil

	repaired, report, err := Repair(faulty, config)
	if err != nil {
		t.Fatalf("Repair() without reference error = %v", err)
	}
	if !report.OK {
		t.Fatalf("repaired matrix failed: %v", report.Issues)
	}
	if MaxMatrixDifference(original, repaired) != 0 {
		t.Fatalf("wrong column repaired: diff=%g", MaxMatrixDifference(original, repaired))
	}
}

func TestRepairRejectsUnlocatableColorError(t *testing.T) {
	original := SRGBToXYZD65()
	broken := original
	broken[2][2] = -broken[2][2]
	config := DefaultConfig()
	if _, _, err := Repair(broken, config); err == nil {
		t.Fatal("Repair accepted a non-axis sign error")
	}
}
