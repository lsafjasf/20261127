package rgbxyz

import (
	"errors"
	"math"
	"testing"
)

func TestInverseRoundTrip(t *testing.T) {
	matrix := SRGBToXYZD65()
	if matrix.Det() <= 0 {
		t.Fatalf("sRGB matrix determinant = %g, want positive", matrix.Det())
	}

	inverse, err := matrix.Inverse()
	if err != nil {
		t.Fatalf("Inverse() error = %v", err)
	}
	if err := RoundTripError(matrix, inverse); err > 1e-12 {
		t.Fatalf("round-trip error = %g, want <= 1e-12", err)
	}
}

func TestSingularMatrixIsRejected(t *testing.T) {
	matrix := Mat3{
		{1, 2, 3},
		{2, 4, 6},
		{0, 1, 1},
	}
	if matrix.InvertibleFor(RelativeTolerance) {
		t.Fatal("dependent rows were reported as invertible")
	}
	if _, err := matrix.Inverse(); !errors.Is(err, ErrSingularMatrix) {
		t.Fatalf("Inverse() error = %v, want ErrSingularMatrix", err)
	}
}

func TestInvertibilityToleranceIsScaleRelative(t *testing.T) {
	small := Identity()
	for row := range small {
		for column := range small[row] {
			small[row][column] *= 1e-100
		}
	}
	if !small.InvertibleFor(RelativeTolerance) {
		t.Fatal("uniformly scaled identity must remain invertible")
	}
	if _, err := small.Inverse(); err != nil {
		t.Fatalf("Inverse() of scaled identity: %v", err)
	}

	config := DefaultConfig()
	config.Reference = nil
	config.WhiteXYZ = config.WhiteXYZ.Scale(1e-100)
	report := DiagnoseWithConfig(small, config)
	if report.Handedness != HandednessRight {
		t.Fatalf("scaled identity handedness = %s, want right-handed", report.Handedness)
	}
	if !report.Invertible {
		t.Fatalf("scaled identity was not invertible: %v", report.Issues)
	}
}

func TestNonFiniteMatrixIsRejected(t *testing.T) {
	matrix := Identity()
	matrix[0][0] = math.NaN()
	if matrix.InvertibleFor(RelativeTolerance) {
		t.Fatal("matrix containing NaN was reported as invertible")
	}
}

func TestFlipColumn(t *testing.T) {
	matrix := SRGBToXYZD65()
	flipped, err := FlipColumn(matrix, 2)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(flipped.Det()+matrix.Det()) > 1e-15 {
		t.Fatalf("determinant sign did not change: original=%g flipped=%g",
			matrix.Det(), flipped.Det())
	}
	if _, err := FlipColumn(matrix, 3); err == nil {
		t.Fatal("FlipColumn accepted an out-of-range column")
	}
}
