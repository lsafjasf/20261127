package observermap

import (
	"errors"
	"math"
)

// XYZ is a CIE 1931 tristimulus value on the Y=100 scale.
type XYZ struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Chromaticity returns CIE xy chromaticity coordinates.
// Returns an error for non-positive tristimulus sum (black/invalid stimulus).
func (c XYZ) Chromaticity() (x, y float64, err error) {
	s := c.X + c.Y + c.Z
	if math.IsInf(s, 0) || math.IsNaN(s) || s <= 0 {
		return 0, 0, errors.New("xy undefined for non-positive X+Y+Z")
	}
	return c.X / s, c.Y / s, nil
}

// Lab is a CIE L*a*b* colour under a chosen reference white.
type Lab struct {
	L float64 `json:"l"`
	A float64 `json:"a"`
	B float64 `json:"b"`
}

const (
	labEpsilon = 216.0 / 24389.0
	labKappa   = 24389.0 / 27.0
)

func labF(t float64) float64 {
	if t > labEpsilon {
		return math.Cbrt(t)
	}
	return (labKappa*t + 16.0) / 116.0
}

// XYZToLab converts XYZ (Y=100 scale) to CIE L*a*b* relative to white.
func XYZToLab(c XYZ, w WhitePoint) Lab {
	fx := labF(c.X / w.XYZ.X)
	fy := labF(c.Y / w.XYZ.Y)
	fz := labF(c.Z / w.XYZ.Z)
	return Lab{
		L: 116*fy - 16,
		A: 500 * (fx - fy),
		B: 200 * (fy - fz),
	}
}

// DeltaE76 is the simple Euclidean CIE76 colour difference.
func DeltaE76(a, b Lab) float64 {
	return math.Hypot(math.Hypot(a.L-b.L, a.A-b.A), a.B-b.B)
}

// DeltaE2000 computes the CIEDE2000 colour difference (kL=kC=kH=1).
//
// Reference: Sharma, Wu, Dalal (2005), "The CIEDE2000 Color-Difference
// Formula", implemented from the published step-by-step equations.
func DeltaE2000(l1, l2 Lab) float64 {
	const kL, kC, kH = 1.0, 1.0, 1.0

	c1 := math.Hypot(l1.A, l1.B)
	c2 := math.Hypot(l2.A, l2.B)
	cBar := (c1 + c2) / 2
	cBar7 := math.Pow(cBar, 7)
	g := 0.5 * (1 - math.Sqrt(cBar7/(cBar7+math.Pow(25, 7))))

	a1p := l1.A * (1 + g)
	a2p := l2.A * (1 + g)
	c1p := math.Hypot(a1p, l1.B)
	c2p := math.Hypot(a2p, l2.B)

	hue := func(ap, b float64) float64 {
		if ap == 0 && b == 0 {
			return 0
		}
		h := math.Atan2(b, ap) * 180 / math.Pi
		if h < 0 {
			h += 360
		}
		return h
	}
	h1p := hue(a1p, l1.B)
	h2p := hue(a2p, l2.B)

	dLp := l2.L - l1.L
	dCp := c2p - c1p

	var dhp float64
	if c1p*c2p != 0 {
		dhp = h2p - h1p
		if dhp > 180 {
			dhp -= 360
		} else if dhp < -180 {
			dhp += 360
		}
	}
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(dhp*math.Pi/360)

	lBarP := (l1.L + l2.L) / 2
	cBarP := (c1p + c2p) / 2

	var hBarP float64
	if c1p*c2p == 0 {
		hBarP = h1p + h2p
	} else {
		sum := h1p + h2p
		if math.Abs(h1p-h2p) > 180 {
			if sum < 360 {
				sum += 360
			} else {
				sum -= 360
			}
		}
		hBarP = sum / 2
	}

	t := 1 -
		0.17*math.Cos(deg2rad(hBarP-30)) +
		0.24*math.Cos(deg2rad(2*hBarP)) +
		0.32*math.Cos(deg2rad(3*hBarP+6)) -
		0.20*math.Cos(deg2rad(4*hBarP-63))

	dTheta := 30 * math.Exp(-math.Pow((hBarP-275)/25, 2))
	cBarP7 := math.Pow(cBarP, 7)
	rC := 2 * math.Sqrt(cBarP7/(cBarP7+math.Pow(25, 7)))
	sL := 1 + (0.015*(lBarP-50)*(lBarP-50))/math.Sqrt(20+(lBarP-50)*(lBarP-50))
	sC := 1 + 0.045*cBarP
	sH := 1 + 0.015*cBarP*t
	rT := -math.Sin(deg2rad(2*dTheta)) * rC

	lTerm := dLp / (kL * sL)
	cTerm := dCp / (kC * sC)
	hTerm := dHp / (kH * sH)
	return math.Sqrt(lTerm*lTerm + cTerm*cTerm + hTerm*hTerm + rT*cTerm*hTerm)
}

func deg2rad(d float64) float64 { return d * math.Pi / 180 }

// SRGBToXYZ converts an sRGB colour (components 0..255) to XYZ under the
// D65 / CIE 1931 2-degree combination assumed by IEC 61966-2-1.
// Returned XYZ uses the Y=100 scale.
func SRGBToXYZ(r8, g8, b8 uint8) XYZ {
	lin := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	r, g, b := lin(r8), lin(g8), lin(b8)
	return XYZ{
		X: 100 * (0.4124564*r + 0.3575761*g + 0.1804375*b),
		Y: 100 * (0.2126729*r + 0.7151522*g + 0.0721750*b),
		Z: 100 * (0.0193339*r + 0.1191920*g + 0.9503041*b),
	}
}

// ParseHexRGB parses "#rgb", "rgb", "#rrggbb" or "rrggbb".
func ParseHexRGB(s string) (r, g, b uint8, err error) {
	h := s
	if len(h) > 0 && h[0] == '#' {
		h = h[1:]
	}
	switch len(h) {
	case 3:
		var v [3]uint8
		for i := 0; i < 3; i++ {
			n, e := parseHexNibble(h[i])
			if e != nil {
				return 0, 0, 0, e
			}
			v[i] = n * 17
		}
		return v[0], v[1], v[2], nil
	case 6:
		var n uint64
		for i := 0; i < 6; i++ {
			d, e := parseHexNibble(h[i])
			if e != nil {
				return 0, 0, 0, e
			}
			n = n<<4 | uint64(d)
		}
		return uint8(n >> 16), uint8(n >> 8), uint8(n), nil
	default:
		return 0, 0, 0, errors.New("hex colour must be 3 or 6 digits")
	}
}

func parseHexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, errors.New("invalid hex digit")
	}
}
