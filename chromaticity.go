package iccproof

import "math"

// XYY is an xy chromaticity coordinate with luminance Y.
type XYY struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	YLevel float64 `json:"Y,omitempty"`
}

func (p XYY) XYZ() XYZ {
	if p.Y == 0 {
		return XYZ{}
	}
	level := p.YLevel
	if level == 0 {
		level = 1
	}
	return XYZ{X: level * p.X / p.Y, Y: level, Z: level * (1 - p.X - p.Y) / p.Y}
}

// D50XYY is the ICC PCS illuminant.
func D50XYY() XYY { return XYY{X: 0.3457, Y: 0.3585, YLevel: 1} }

// D65XYY is the common display/daylight white.
func D65XYY() XYY { return XYY{X: 0.3127, Y: 0.3290, YLevel: 1} }

// PrimariesMatrix computes the linear RGB -> XYZ matrix for xy primaries.
func PrimariesMatrix(red, green, blue, white XYY) [3][3]float64 {
	xr, yr := red.X, red.Y
	xg, yg := green.X, green.Y
	xb, yb := blue.X, blue.Y

	m := [3][3]float64{
		{xr / yr, xg / yg, xb / yb},
		{1, 1, 1},
		{(1 - xr - yr) / yr, (1 - xg - yg) / yg, (1 - xb - yb) / yb},
	}
	minv := inverse3(m)
	w := white.XYZ()
	ws := [3]float64{w.X, w.Y, w.Z}
	s := matVec3(minv, ws)
	return [3][3]float64{
		{m[0][0] * s[0], m[0][1] * s[1], m[0][2] * s[2]},
		{m[1][0] * s[0], m[1][1] * s[1], m[1][2] * s[2]},
		{m[2][0] * s[0], m[2][1] * s[1], m[2][2] * s[2]},
	}
}

func matVec3(m [3][3]float64, v [3]float64) [3]float64 {
	return [3]float64{
		m[0][0]*v[0] + m[0][1]*v[1] + m[0][2]*v[2],
		m[1][0]*v[0] + m[1][1]*v[1] + m[1][2]*v[2],
		m[2][0]*v[0] + m[2][1]*v[1] + m[2][2]*v[2],
	}
}

func inverse3(a [3][3]float64) [3][3]float64 {
	det := a[0][0]*(a[1][1]*a[2][2]-a[1][2]*a[2][1]) -
		a[0][1]*(a[1][0]*a[2][2]-a[1][2]*a[2][0]) +
		a[0][2]*(a[1][0]*a[2][1]-a[1][1]*a[2][0])
	if math.Abs(det) < 1e-15 {
		return [3][3]float64{}
	}
	id := 1 / det
	return [3][3]float64{
		{(a[1][1]*a[2][2] - a[1][2]*a[2][1]) * id, (a[0][2]*a[2][1] - a[0][1]*a[2][2]) * id, (a[0][1]*a[1][2] - a[0][2]*a[1][1]) * id},
		{(a[1][2]*a[2][0] - a[1][0]*a[2][2]) * id, (a[0][0]*a[2][2] - a[0][2]*a[2][0]) * id, (a[0][2]*a[1][0] - a[0][0]*a[1][2]) * id},
		{(a[1][0]*a[2][1] - a[1][1]*a[2][0]) * id, (a[0][1]*a[2][0] - a[0][0]*a[2][1]) * id, (a[0][0]*a[1][1] - a[0][1]*a[1][0]) * id},
	}
}

// LabWhite returns the white point in the Y=100 convention expected by XYZToLab.
func LabWhite(w XYY) XYZ {
	xyz := w.XYZ()
	return XYZ{X: xyz.X * 100, Y: xyz.Y * 100, Z: xyz.Z * 100}
}

// D50LabWhite is the ICC PCS white in Y=100 units.
func D50LabWhite() XYZ { return LabWhite(D50XYY()) }
