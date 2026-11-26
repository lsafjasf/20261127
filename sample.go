package iccproof

import "fmt"

// Sample is one photo pixel, patch, or external color measurement.
//
// Encoded RGB values are normalized to 0..1. CodeRGB/BitDepth can be used
// instead; code values are converted with the declared bit depth. Lab/XYZ may
// describe an ideal reference color whose representability by the source
// profile should be checked.
type Sample struct {
	Index    int       `json:"index,omitempty"`
	ID       string    `json:"id,omitempty"`
	X        *int      `json:"x,omitempty"`
	Y        *int      `json:"y,omitempty"`
	RGB      []float64 `json:"rgb,omitempty"`
	CodeRGB  []int     `json:"code_rgb,omitempty"`
	BitDepth int       `json:"bit_depth,omitempty"`
	Lab      *Lab      `json:"lab,omitempty"`
	XYZ      *XYZ      `json:"xyz,omitempty"`
}

// NormalizedRGB returns encoded RGB in 0..1 and validates source encoding
// limits. The returned violations describe source-side unrepresentability.
func (s Sample) NormalizedRGB() ([3]float64, []ChannelBoundary, error) {
	var rgb [3]float64
	switch {
	case len(s.RGB) == 3:
		copy(rgb[:], s.RGB)
	case len(s.CodeRGB) == 3:
		bits := s.BitDepth
		if bits == 0 {
			bits = 8
		}
		if bits != 8 && bits != 10 && bits != 12 && bits != 16 {
			return rgb, nil, fmt.Errorf("unsupported bit_depth %d", bits)
		}
		maxCode := float64(int(1)<<bits - 1)
		for i, code := range s.CodeRGB {
			if code < 0 || float64(code) > maxCode {
				rgb[i] = float64(code) / maxCode
				continue
			}
			rgb[i] = float64(code) / maxCode
		}
	default:
		if s.Lab == nil && s.XYZ == nil {
			return rgb, nil, fmt.Errorf("sample requires rgb[3], code_rgb[3], lab, or xyz")
		}
		// Reference-color samples are checked against the source profile by
		// the engine; an empty encoded RGB tuple is valid here.
		return rgb, nil, nil
	}

	var violations []ChannelBoundary
	names := []string{"R", "G", "B"}
	for i, v := range rgb {
		if v < -DomainTolerance || v > 1+DomainTolerance {
			min, max := 0.0, 1.0
			violations = append(violations, ChannelBoundary{
				Stage: BoundarySource, Channel: i, ChannelName: names[i],
				Value: v, Min: min, Max: max,
				Exceedance: maxFloat(min-v, v-max),
				Reason:     ReasonSourceCodeOutOfRange,
			})
		}
	}
	return rgb, violations, nil
}

const DomainTolerance = 1e-7

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// Coordinate returns x,y or -1,-1 when absent.
func (s Sample) Coordinate() (int, int) {
	x, y := -1, -1
	if s.X != nil {
		x = *s.X
	}
	if s.Y != nil {
		y = *s.Y
	}
	return x, y
}
