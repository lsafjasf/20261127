package iccproof

import "math"

// Lab is a CIE L*a*b* color. L is normally 0..100, a/b are unbounded.
type Lab struct {
	L float64 `json:"L"`
	A float64 `json:"a"`
	B float64 `json:"b"`
}

// XYZ uses the common Y=100 convention.
type XYZ struct {
	X float64 `json:"X"`
	Y float64 `json:"Y"`
	Z float64 `json:"Z"`
}

// DeltaE76 is plain Euclidean Lab distance.
func DeltaE76(a, b Lab) float64 {
	return math.Sqrt(square(a.L-b.L) + square(a.A-b.A) + square(a.B-b.B))
}

// DeltaE00 implements CIEDE2000 (CIE 142-2001). The return value is dimensionless
// and is the primary per-sample color difference in reports.
func DeltaE00(lab1, lab2 Lab) float64 {
	const (
		kl = 1.0
		kc = 1.0
		kh = 1.0
	)

	l1, a1, b1 := lab1.L, lab1.A, lab1.B
	l2, a2, b2 := lab2.L, lab2.A, lab2.B

	c1ab := math.Hypot(a1, b1)
	c2ab := math.Hypot(a2, b2)
	cab := (c1ab + c2ab) / 2

	cab7 := math.Pow(cab, 7)
	g := 0.5 * (1 - math.Sqrt(cab7/(cab7+math.Pow(25, 7))))
	a1p := (1 + g) * a1
	a2p := (1 + g) * a2
	c1p := math.Hypot(a1p, b1)
	c2p := math.Hypot(a2p, b2)

	h1p := hueAngle(a1p, b1)
	h2p := hueAngle(a2p, b2)

	dlp := l2 - l1
	dcp := c2p - c1p

	var dhp float64
	switch {
	case c1p*c2p == 0:
		dhp = 0
	case h2p-h1p > 180:
		dhp = h2p - h1p - 360
	case h2p-h1p < -180:
		dhp = h2p - h1p + 360
	default:
		dhp = h2p - h1p
	}
	dhpMetric := 2 * math.Sqrt(c1p*c2p) * sind(dhp/2)

	var hbp float64
	switch {
	case c1p*c2p == 0:
		hbp = h1p + h2p
	case math.Abs(h1p-h2p) <= 180:
		hbp = (h1p + h2p) / 2
	case h1p+h2p < 360:
		hbp = (h1p + h2p + 360) / 2
	default:
		hbp = (h1p + h2p - 360) / 2
	}

	lbp := (l1 + l2) / 2
	cbp := (c1p + c2p) / 2
	t := 1 -
		0.17*cosd(hbp-30) +
		0.24*cosd(2*hbp) +
		0.32*cosd(3*hbp+6) -
		0.20*cosd(4*hbp-63)

	dtheta := 30 * math.Exp(-square((hbp-275)/25))
	rc := 2 * math.Sqrt(math.Pow(cbp, 7)/(math.Pow(cbp, 7)+math.Pow(25, 7)))
	sl := 1 + (0.015*square(lbp-50))/math.Sqrt(20+square(lbp-50))
	sc := 1 + 0.045*cbp
	sh := 1 + 0.015*cbp*t
	rt := -sind(2*dtheta) * rc

	return math.Sqrt(
		square(dlp/(kl*sl)) +
			square(dcp/(kc*sc)) +
			square(dhpMetric/(kh*sh)) +
			rt*(dcp/(kc*sc))*(dhpMetric/(kh*sh)),
	)
}

func hueAngle(a, b float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

func square(v float64) float64 { return v * v }

func sind(deg float64) float64 { return math.Sin(deg * math.Pi / 180) }
func cosd(deg float64) float64 { return math.Cos(deg * math.Pi / 180) }

// RGBToXYZLinear converts normalized linear RGB using explicit primaries and
// white point. It is used by the non-cgo reference engine and tests.
func RGBToXYZLinear(rgb [3]float64, r, g, b, w XYY) XYZ {
	m := PrimariesMatrix(r, g, b, w)
	return XYZ{
		X: 100 * (m[0][0]*rgb[0] + m[0][1]*rgb[1] + m[0][2]*rgb[2]),
		Y: 100 * (m[1][0]*rgb[0] + m[1][1]*rgb[1] + m[1][2]*rgb[2]),
		Z: 100 * (m[2][0]*rgb[0] + m[2][1]*rgb[1] + m[2][2]*rgb[2]),
	}
}

// XYZToLab converts Y=100 XYZ relative to white XYZ.
func XYZToLab(xyz XYZ, white XYZ) Lab {
	fx := labF(xyz.X / white.X)
	fy := labF(xyz.Y / white.Y)
	fz := labF(xyz.Z / white.Z)
	return Lab{L: 116*fy - 16, A: 500 * (fx - fy), B: 200 * (fy - fz)}
}

func labF(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta*delta*delta {
		return math.Cbrt(t)
	}
	return t/(3*delta*delta) + 4.0/29.0
}

// SRGBEOTF decodes an sRGB/BT.709 encoded channel to linear light.
func SRGBEOTF(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// SRGBOETF encodes a linear channel. Out-of-range values are preserved here;
// callers decide whether that is a boundary condition or clipping.
func SRGBOETF(v float64) float64 {
	if v <= 0.0031308 {
		return 12.92 * v
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}
