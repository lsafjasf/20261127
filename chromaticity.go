package gamutcompare

import (
	"errors"
	"fmt"
	"math"
)

// ValidateChromaticities checks geometric/input validity. Coordinates outside
// the physical CIE xy region are reported as warnings elsewhere; synthetic
// geometry samples remain valid test inputs.
func ValidateChromaticities(points ...Point) error {
	for i, p := range points {
		if !finitePoint(p) {
			return fmt.Errorf("point %d must be finite", i)
		}
	}
	return nil
}

func xyPhysicalWarning(label string, p Point) string {
	if p.X < -geometryEps || p.Y < -geometryEps || p.X+p.Y > 1+1e-7 {
		return fmt.Sprintf("%s=(%.6f,%.6f) is outside the physical CIE xy half-plane region", label, p.X, p.Y)
	}
	return ""
}

// WhiteInfo describes whether the supplied white can be an additive mixture of
// the RGB primaries. Shares are normalized to Y_white=1.
type WhiteInfo struct {
	Inside          bool       `json:"inside"`
	LuminanceShares [3]float64 `json:"relative_luminance_shares"`
	NegativeShare   float64    `json:"negative_share,omitempty"`
	IllConditioned  bool       `json:"ill_conditioned,omitempty"`
}

// PrimaryLuminanceShares solves XYZ(RGB scales)=XYZ(white), with Y_white=1.
func PrimaryLuminanceShares(red, green, blue, white Point) (WhiteInfo, error) {
	columns := [3][3]float64{
		xyzUnitY(red),
		xyzUnitY(green),
		xyzUnitY(blue),
	}
	matrix := [3][3]float64{
		{columns[0][0], columns[1][0], columns[2][0]},
		{columns[0][1], columns[1][1], columns[2][1]},
		{columns[0][2], columns[1][2], columns[2][2]},
	}
	target := xyzUnitY(white)

	shares, ok := solve3(matrix, target)
	if !ok {
		return WhiteInfo{IllConditioned: true}, errors.New("RGB-to-XYZ matrix is singular or ill-conditioned")
	}

	var negative float64
	for _, v := range shares {
		if v < -geometryEps {
			negative = math.Max(negative, -v)
		}
	}
	info := WhiteInfo{
		Inside:          negative == 0,
		LuminanceShares: shares,
		NegativeShare:   negative,
	}
	return info, nil
}

func xyzUnitY(p Point) [3]float64 {
	if math.Abs(p.Y) < geometryEps {
		return [3]float64{math.Inf(1), 1, math.Inf(1)}
	}
	return [3]float64{p.X / p.Y, 1, (1 - p.X - p.Y) / p.Y}
}

func solve3(a [3][3]float64, b [3]float64) ([3]float64, bool) {
	const eps = 1e-14
	for col := 0; col < 3; col++ {
		pivot := col
		for row := col + 1; row < 3; row++ {
			if math.Abs(a[row][col]) > math.Abs(a[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(a[pivot][col]) < eps {
			return [3]float64{}, false
		}
		if pivot != col {
			a[col], a[pivot] = a[pivot], a[col]
			b[col], b[pivot] = b[pivot], b[col]
		}
		for row := col + 1; row < 3; row++ {
			factor := a[row][col] / a[col][col]
			for k := col; k < 3; k++ {
				a[row][k] -= factor * a[col][k]
			}
			b[row] -= factor * b[col]
		}
	}

	var x [3]float64
	for row := 2; row >= 0; row-- {
		sum := b[row]
		for k := row + 1; k < 3; k++ {
			sum -= a[row][k] * x[k]
		}
		x[row] = sum / a[row][row]
		if !isFiniteFloat(x[row]) {
			return x, false
		}
	}
	return x, true
}

func gamutAreas(tri []Point) (areaXY, areaUpVP float64) {
	uv := make([]Point, len(tri))
	for i, p := range tri {
		uv[i] = p.upVPPoint()
	}
	return PolygonArea(tri), PolygonArea(uv)
}
