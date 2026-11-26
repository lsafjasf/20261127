package colorcompare

import "math"

var d65 = [3]float64{0.95047, 1.00000, 1.08883}

// LabFromRGB converts an 8-bit sRGB color to CIE L*a*b* using D65 and the
// standard IEC 61966-2-1 transfer function.
func LabFromRGB(c RGB24) Lab {
	r := srgbToLinear(float64(c.R) / 255)
	g := srgbToLinear(float64(c.G) / 255)
	b := srgbToLinear(float64(c.B) / 255)

	x := 0.4124564*r + 0.3575761*g + 0.1804375*b
	y := 0.2126729*r + 0.7151522*g + 0.0721750*b
	z := 0.0193339*r + 0.1191920*g + 0.9503041*b

	fx := labF(x / d65[0])
	fy := labF(y / d65[1])
	fz := labF(z / d65[2])
	return Lab{
		L: 116*fy - 16,
		A: 500 * (fx - fy),
		B: 200 * (fy - fz),
	}
}

func srgbToLinear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func labF(t float64) float64 {
	const epsilon = 216.0 / 24389.0
	const kappa = 24389.0 / 27.0
	if t > epsilon {
		return math.Cbrt(t)
	}
	return (kappa*t + 16) / 116
}
