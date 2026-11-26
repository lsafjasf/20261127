package colorcompare

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RGB24 is an 8-bit-per-channel sRGB color, matching the common RGB24 layout.
type RGB24 struct {
	R uint8
	G uint8
	B uint8
}

// Hex returns the color in #RRGGBB form.
func (c RGB24) Hex() string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// ParseRGB24 parses #RRGGBB or RRGGBB into an RGB24 value.
func ParseRGB24(s string) (RGB24, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return RGB24{}, fmt.Errorf("RGB24 must contain exactly 6 hex digits, got %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 24)
	if err != nil {
		return RGB24{}, fmt.Errorf("invalid RGB24 %q: %w", s, err)
	}
	return RGB24{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
	}, nil
}

// Lab is a CIE L*a*b* value under the D65 white point.
type Lab struct {
	L float64
	A float64
	B float64
}

// EuclideanRGB returns the ordinary, unnormalized Euclidean distance in the
// 0..255 RGB cube. It deliberately applies no contrast weighting or scaling.
func EuclideanRGB(a, b RGB24) float64 {
	dr := float64(a.R) - float64(b.R)
	dg := float64(a.G) - float64(b.G)
	db := float64(a.B) - float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// EuclideanLab returns CIE76 distance in Lab: the ordinary Euclidean distance
// after a perceptual color-space transform.
func EuclideanLab(a, b Lab) float64 {
	dl := a.L - b.L
	da := a.A - b.A
	db := a.B - b.B
	return math.Sqrt(dl*dl + da*da + db*db)
}
