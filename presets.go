package gamutcompare

import (
	"fmt"
	"strings"
)

var d65 = Point{X: 0.31270, Y: 0.32900}

// TargetPreset returns common CIE xy target gamuts. These are chromaticity
// targets only; transfer functions and luminance are intentionally absent.
func TargetPreset(name string) (TargetSpec, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "srgb", "bt709", "rec709":
		return TargetSpec{
			Name:  "sRGB / BT.709",
			White: pointPtr(d65),
			Polygon: []Point{
				{X: 0.640, Y: 0.330},
				{X: 0.300, Y: 0.600},
				{X: 0.150, Y: 0.060},
			},
		}, nil
	case "display-p3", "p3", "dci-p3":
		white := d65
		if name == "dci-p3" {
			white = Point{X: 0.314, Y: 0.351}
		}
		return TargetSpec{
			Name:  "Display P3 / DCI-P3 primaries",
			White: pointPtr(white),
			Polygon: []Point{
				{X: 0.680, Y: 0.320},
				{X: 0.265, Y: 0.690},
				{X: 0.150, Y: 0.060},
			},
		}, nil
	case "adobergb", "adobe-rgb":
		return TargetSpec{
			Name:  "Adobe RGB (1998)",
			White: pointPtr(d65),
			Polygon: []Point{
				{X: 0.6400, Y: 0.3300},
				{X: 0.2100, Y: 0.7100},
				{X: 0.1500, Y: 0.0600},
			},
		}, nil
	case "bt2020", "rec2020":
		return TargetSpec{
			Name:  "BT.2020 / Rec.2020",
			White: pointPtr(d65),
			Polygon: []Point{
				{X: 0.708, Y: 0.292},
				{X: 0.170, Y: 0.797},
				{X: 0.131, Y: 0.046},
			},
		}, nil
	default:
		return TargetSpec{}, fmt.Errorf("unknown target preset %q", name)
	}
}

func pointPtr(p Point) *Point { return &p }
