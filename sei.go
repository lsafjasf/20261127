package hdrswitch

import "math"

// SEIPrimaryCoordinate converts a HEVC mastering_display_colour_volume
// chromaticity code to CIE xy. Codes are in units of 0.00002 = 1/50000.
func SEIPrimaryCoordinate(code uint16) float64 {
	return float64(code) / 50000.0
}

// QuantizeSEIPrimaryCoordinate rounds CIE xy to the HEVC SEI uint16 code.
func QuantizeSEIPrimaryCoordinate(coordinate float64) uint16 {
	return uint16(math.Round(coordinate * 50000.0))
}

// SEILuminanceNits converts a HEVC mastering display luminance code to nits.
// Luminance codes are in units of 0.0001 cd/m² = 1/10000 nit.
func SEILuminanceNits(code uint32) float64 {
	return float64(code) / 10000.0
}

// QuantizeSEILuminance rounds nits to the HEVC SEI uint32 luminance code.
func QuantizeSEILuminance(nits float64) uint32 {
	return uint32(math.Round(nits * 10000.0))
}

// PQLuminanceNits evaluates SMPTE ST 2084's inverse OETF for normalized
// PQ signal N in [0,1]. N=1 maps to 10000 nits.
func PQLuminanceNits(signal float64) float64 {
	const (
		m1 = 2610.0 / 16384.0
		m2 = 2523.0 / 4096.0 * 128.0
		c1 = 3424.0 / 4096.0
		c2 = 2413.0 / 4096.0 * 32.0
		c3 = 2392.0 / 4096.0 * 32.0
	)
	if signal <= 0 {
		return 0
	}
	if signal >= 1 {
		return 10000
	}
	power := math.Pow(signal, 1.0/m2)
	numerator := math.Max(power-c1, 0)
	denominator := c2 - c3*power
	linear := math.Pow(numerator/denominator, 1.0/m1)
	return 10000 * linear
}

// HLGSystemGamma returns the BT.2100 reference OOTF gamma for a nominal
// display peak Lw in nits.
func HLGSystemGamma(nominalPeakNits float64) float64 {
	if nominalPeakNits <= 0 {
		return 1.2
	}
	return 1.2 + 0.42*math.Log10(nominalPeakNits/1000.0)
}
