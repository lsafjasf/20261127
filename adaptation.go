package iccproof

import "math"

// AdaptXYZ applies Bradford chromatic adaptation between white points.
// XYZ values use Y=100.
func AdaptXYZ(xyz XYZ, from, to XYY) XYZ {
	if from == to || from.Y <= 0 || to.Y <= 0 {
		return xyz
	}
	wf := from.XYZ()
	wt := to.XYZ()
	rf := bradford([3]float64{wf.X, wf.Y, wf.Z})
	rt := bradford([3]float64{wt.X, wt.Y, wt.Z})
	if math.Abs(rf[0]) < 1e-12 || math.Abs(rf[1]) < 1e-12 || math.Abs(rf[2]) < 1e-12 {
		return xyz
	}
	scale := [3]float64{rt[0] / rf[0], rt[1] / rf[1], rt[2] / rf[2]}
	m := bradfordM
	minv := inverse3(bradfordM)
	cone := bradford([3]float64{xyz.X, xyz.Y, xyz.Z})
	scaled := [3]float64{cone[0] * scale[0], cone[1] * scale[1], cone[2] * scale[2]}
	out := matVec3(minv, scaled)
	_ = m
	return XYZ{X: out[0], Y: out[1], Z: out[2]}
}

var bradfordM = [3][3]float64{
	{0.8951, 0.2664, -0.1614},
	{-0.7502, 1.7135, 0.0367},
	{0.0389, -0.0685, 1.0296},
}

func bradford(xyz [3]float64) [3]float64 {
	return matVec3(bradfordM, xyz)
}

// LabToXYZD50 converts CIE Lab relative to D50 to XYZ Y=100.
func LabToXYZD50(lab Lab) XYZ {
	white := D50XYY().XYZ()
	fy := (lab.L + 16) / 116
	fx := lab.A/500 + fy
	fz := fy - lab.B/200
	x := labInvF(fx) * white.X
	y := labInvF(fy) * white.Y
	z := labInvF(fz) * white.Z
	return XYZ{X: x, Y: y, Z: z}
}

func labInvF(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta {
		return t * t * t
	}
	return 3 * delta * delta * (t - 4.0/29.0)
}
