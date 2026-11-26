package rgbxyz

import (
	"reflect"
	"testing"
)

func TestCompareColorsIdenticalMatrices(t *testing.T) {
	matrix := SRGBToXYZD65()
	stats := CompareColors(matrix, matrix, SampleColors(), D65White())
	if stats.Samples != len(SampleColors()) {
		t.Fatalf("samples = %d, want %d", stats.Samples, len(SampleColors()))
	}
	if stats.MaxXYZ != 0 || stats.MeanXYZ != 0 ||
		stats.MaxDeltaE != 0 || stats.MeanDeltaE != 0 {
		t.Fatalf("identical matrices produced non-zero error: %+v", stats)
	}
}

func TestCompareColorsDetectsReflection(t *testing.T) {
	original := SRGBToXYZD65()
	faulty, err := FlipColumn(original, 2)
	if err != nil {
		t.Fatal(err)
	}
	stats := CompareColors(original, faulty, SampleColors(), D65White())
	if stats.MaxXYZ <= 1e-3 || stats.MaxDeltaE <= 1 {
		t.Fatalf("reflection was not detected: %+v", stats)
	}
}

func TestMinimalCounterexampleIsOneBlueCode(t *testing.T) {
	original := SRGBToXYZD65()
	faulty, err := FlipColumn(original, 2)
	if err != nil {
		t.Fatal(err)
	}
	counterexample, ok := FindMinimalCounterexample(
		faulty, original, D65White(), 1e-6,
	)
	if !ok {
		t.Fatal("no counterexample found")
	}
	want := [3]uint8{0, 0, 1}
	if !reflect.DeepEqual(counterexample.RGB8, want) {
		t.Fatalf("RGB8 = %v, want %v", counterexample.RGB8, want)
	}
	if counterexample.Hex() != "#000001" {
		t.Fatalf("hex = %s, want #000001", counterexample.Hex())
	}
	if counterexample.DeltaE <= 1e-6 || counterexample.XYZError <= 1e-6 {
		t.Fatalf("counterexample error too small: %+v", counterexample)
	}
}

func TestErrorStatsEquality(t *testing.T) {
	a := ColorErrorStats{
		Samples: 2, MaxXYZ: 0.1, MeanXYZ: 0.05, MaxDeltaE: 2, MeanDeltaE: 1,
	}
	b := a
	if !ErrorStatsEqual(a, b, 0) {
		t.Fatal("identical statistics compared unequal")
	}
	b.MaxDeltaE += 1e-3
	if ErrorStatsEqual(a, b, 1e-4) {
		t.Fatal("different statistics compared equal")
	}
}
