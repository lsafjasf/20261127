// Package rgbxyz validates and repairs 3x3 RGB-to-XYZ color matrices.
//
// Mat3 uses row-major storage: m[row][column]. A color vector is treated as a
// column vector, so the conversion is xyz = m * rgb.
package rgbxyz

import (
	"errors"
	"fmt"
	"math"
)

// RelativeTolerance is the default tolerance used for determinant and
// inversion checks.
const RelativeTolerance = 1e-12

// ErrSingularMatrix is returned when a matrix cannot be inverted reliably.
var ErrSingularMatrix = errors.New("singular or numerically unstable matrix")

// Vec3 is a three-component column vector.
type Vec3 [3]float64

// Add returns v + other.
func (v Vec3) Add(other Vec3) Vec3 {
	return Vec3{v[0] + other[0], v[1] + other[1], v[2] + other[2]}
}

// Sub returns v - other.
func (v Vec3) Sub(other Vec3) Vec3 {
	return Vec3{v[0] - other[0], v[1] - other[1], v[2] - other[2]}
}

// Scale returns factor*v.
func (v Vec3) Scale(factor float64) Vec3 {
	return Vec3{v[0] * factor, v[1] * factor, v[2] * factor}
}

// Dot returns the dot product of v and other.
func (v Vec3) Dot(other Vec3) float64 {
	return v[0]*other[0] + v[1]*other[1] + v[2]*other[2]
}

// Cross returns v x other.
func (v Vec3) Cross(other Vec3) Vec3 {
	return Vec3{
		v[1]*other[2] - v[2]*other[1],
		v[2]*other[0] - v[0]*other[2],
		v[0]*other[1] - v[1]*other[0],
	}
}

// Norm returns the Euclidean length of v.
func (v Vec3) Norm() float64 {
	return math.Sqrt(v.Dot(v))
}

// IsFinite reports whether every component is finite.
func (v Vec3) IsFinite() bool {
	for _, component := range v {
		if math.IsNaN(component) || math.IsInf(component, 0) {
			return false
		}
	}
	return true
}

// Mat3 is a row-major 3x3 matrix.
type Mat3 [3][3]float64

// Identity returns the 3x3 identity matrix.
func Identity() Mat3 {
	return Mat3{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
}

// Row returns the i-th row.
func (m Mat3) Row(i int) Vec3 {
	return Vec3{m[i][0], m[i][1], m[i][2]}
}

// Col returns the i-th column.
func (m Mat3) Col(i int) Vec3 {
	return Vec3{m[0][i], m[1][i], m[2][i]}
}

// MulVec returns m*v.
func (m Mat3) MulVec(v Vec3) Vec3 {
	return Vec3{m.Row(0).Dot(v), m.Row(1).Dot(v), m.Row(2).Dot(v)}
}

// Mul returns m*other.
func (m Mat3) Mul(other Mat3) Mat3 {
	var product Mat3
	for row := 0; row < 3; row++ {
		for column := 0; column < 3; column++ {
			product[row][column] = m.Row(row).Dot(other.Col(column))
		}
	}
	return product
}

// Transpose returns m transposed.
func (m Mat3) Transpose() Mat3 {
	var transposed Mat3
	for row := 0; row < 3; row++ {
		for column := 0; column < 3; column++ {
			transposed[column][row] = m[row][column]
		}
	}
	return transposed
}

// Det returns the determinant of m.
func (m Mat3) Det() float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// MaxAbs returns the largest absolute matrix entry.
func (m Mat3) MaxAbs() float64 {
	var maximum float64
	for row := 0; row < 3; row++ {
		for column := 0; column < 3; column++ {
			maximum = math.Max(maximum, math.Abs(m[row][column]))
		}
	}
	return maximum
}

// DeterminantToleranceFor returns the smallest determinant magnitude treated
// as non-zero for the given relative tolerance.
func (m Mat3) DeterminantToleranceFor(relativeTolerance float64) float64 {
	scale := m.MaxAbs()
	return relativeTolerance * scale * scale * scale
}

// IsFinite reports whether every matrix entry is finite.
func (m Mat3) IsFinite() bool {
	for row := 0; row < 3; row++ {
		for column := 0; column < 3; column++ {
			if math.IsNaN(m[row][column]) || math.IsInf(m[row][column], 0) {
				return false
			}
		}
	}
	return true
}

// InvertibleFor reports whether m is invertible using a scale-relative
// determinant threshold.
func (m Mat3) InvertibleFor(relativeTolerance float64) bool {
	if !m.IsFinite() {
		return false
	}
	scale := m.MaxAbs()
	if scale == 0 {
		return false
	}
	return math.Abs(m.Det()) > m.DeterminantToleranceFor(relativeTolerance)
}

// Inverse returns m inverse. It refuses matrices whose determinants are too
// small relative to the matrix scale.
func (m Mat3) Inverse() (Mat3, error) {
	return m.InverseFor(RelativeTolerance)
}

// InverseFor is Inverse with an explicit relative tolerance.
func (m Mat3) InverseFor(relativeTolerance float64) (Mat3, error) {
	if !m.InvertibleFor(relativeTolerance) {
		return Mat3{}, ErrSingularMatrix
	}

	determinant := m.Det()
	inverse := Mat3{
		{
			(m[1][1]*m[2][2] - m[1][2]*m[2][1]) / determinant,
			(m[0][2]*m[2][1] - m[0][1]*m[2][2]) / determinant,
			(m[0][1]*m[1][2] - m[0][2]*m[1][1]) / determinant,
		},
		{
			(m[1][2]*m[2][0] - m[1][0]*m[2][2]) / determinant,
			(m[0][0]*m[2][2] - m[0][2]*m[2][0]) / determinant,
			(m[0][2]*m[1][0] - m[0][0]*m[1][2]) / determinant,
		},
		{
			(m[1][0]*m[2][1] - m[1][1]*m[2][0]) / determinant,
			(m[0][1]*m[2][0] - m[0][0]*m[2][1]) / determinant,
			(m[0][0]*m[1][1] - m[0][1]*m[1][0]) / determinant,
		},
	}
	if !inverse.IsFinite() {
		return Mat3{}, ErrSingularMatrix
	}
	return inverse, nil
}

// MaxDifference returns the largest absolute component difference between two
// vectors.
func MaxDifference(a, b Vec3) float64 {
	var maximum float64
	for i := range a {
		maximum = math.Max(maximum, math.Abs(a[i]-b[i]))
	}
	return maximum
}

// MaxMatrixDifference returns the largest absolute entry difference.
func MaxMatrixDifference(a, b Mat3) float64 {
	var maximum float64
	for row := 0; row < 3; row++ {
		for column := 0; column < 3; column++ {
			maximum = math.Max(maximum, math.Abs(a[row][column]-b[row][column]))
		}
	}
	return maximum
}

// RoundTripError returns the largest absolute error of m*inverse(m) and
// inverse(m)*m against the identity.
func RoundTripError(m Mat3, inverse Mat3) float64 {
	left := MaxMatrixDifference(m.Mul(inverse), Identity())
	right := MaxMatrixDifference(inverse.Mul(m), Identity())
	return math.Max(left, right)
}

// FlipColumn returns a copy of m with column index multiplied by -1.
func FlipColumn(m Mat3, index int) (Mat3, error) {
	if index < 0 || index > 2 {
		return Mat3{}, fmt.Errorf("column index %d out of range", index)
	}
	for row := 0; row < 3; row++ {
		m[row][index] = -m[row][index]
	}
	return m, nil
}
