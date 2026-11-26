package rgbxyz

// SRGBToXYZD65 returns the IEC 61966-2-1 linear-sRGB to CIE XYZ matrix for
// the D65 reference white.
func SRGBToXYZD65() Mat3 {
	return Mat3{
		{0.4124564, 0.3575761, 0.1804375},
		{0.2126729, 0.7151522, 0.0721750},
		{0.0193339, 0.1191920, 0.9503041},
	}
}

// D65White returns the CIE XYZ coordinates of the D65 reference white.
func D65White() Vec3 {
	return Vec3{0.95047, 1.00000, 1.08883}
}

// RGBWhite returns normalized linear-RGB white, (1, 1, 1).
func RGBWhite() Vec3 {
	return Vec3{1, 1, 1}
}

// SampleColors returns a deterministic set of colors used for accuracy checks.
// Values are linear RGB in the range [0, 1].
func SampleColors() []Vec3 {
	return []Vec3{
		{1, 0, 0}, {0, 1, 0}, {0, 0, 1},
		{1, 1, 0}, {1, 0, 1}, {0, 1, 1},
		{1, 1, 1}, {0.5, 0.5, 0.5},
		{0.25, 0.5, 0.75}, {0.9, 0.2, 0.1},
	}
}
