package rgbxyz

import (
	"fmt"
	"math"
)

// ColorErrorStats summarizes the difference between two RGB-to-XYZ mappings.
type ColorErrorStats struct {
	Samples    int
	MaxXYZ     float64
	MeanXYZ    float64
	MaxDeltaE  float64
	MeanDeltaE float64
}

// CompareColors converts each linear-RGB sample through both matrices and
// reports CIE76 Delta-E (XYZ) and XYZ Euclidean error.
func CompareColors(reference, candidate Mat3, colors []Vec3, whiteXYZ Vec3) ColorErrorStats {
	if len(colors) == 0 {
		colors = SampleColors()
	}
	if whiteXYZ == (Vec3{}) {
		whiteXYZ = D65White()
	}

	stats := ColorErrorStats{Samples: len(colors)}
	var sumXYZ, sumDeltaE float64
	for _, rgb := range colors {
		referenceXYZ := reference.MulVec(rgb)
		candidateXYZ := candidate.MulVec(rgb)
		xyzError := referenceXYZ.Sub(candidateXYZ).Norm()
		deltaE := deltaE76(referenceXYZ, candidateXYZ, whiteXYZ)
		stats.MaxXYZ = math.Max(stats.MaxXYZ, xyzError)
		stats.MaxDeltaE = math.Max(stats.MaxDeltaE, deltaE)
		sumXYZ += xyzError
		sumDeltaE += deltaE
	}
	stats.MeanXYZ = sumXYZ / float64(len(colors))
	stats.MeanDeltaE = sumDeltaE / float64(len(colors))
	return stats
}

// ErrorStatsEqual reports whether two error summaries agree within tolerance.
func ErrorStatsEqual(a, b ColorErrorStats, tolerance float64) bool {
	return a.Samples == b.Samples &&
		math.Abs(a.MaxXYZ-b.MaxXYZ) <= tolerance &&
		math.Abs(a.MeanXYZ-b.MeanXYZ) <= tolerance &&
		math.Abs(a.MaxDeltaE-b.MaxDeltaE) <= tolerance &&
		math.Abs(a.MeanDeltaE-b.MeanDeltaE) <= tolerance
}

// Counterexample is the smallest quantized linear-RGB color that exposes a
// difference between two matrices.
type Counterexample struct {
	RGB8        [3]uint8
	RGB         Vec3
	FaultyXYZ   Vec3
	RepairedXYZ Vec3
	XYZError    float64
	DeltaE      float64
}

// Hex returns the 8-bit color as #RRGGBB.
func (c Counterexample) Hex() string {
	return fmt.Sprintf("#%02X%02X%02X", c.RGB8[0], c.RGB8[1], c.RGB8[2])
}

// FindMinimalCounterexample searches 8-bit linear-RGB values in increasing L1
// distance from black and returns the first color whose CIE76 Delta-E exceeds
// threshold.
func FindMinimalCounterexample(faulty, repaired Mat3, whiteXYZ Vec3, threshold float64) (Counterexample, bool) {
	if threshold <= 0 {
		threshold = 1e-6
	}
	if whiteXYZ == (Vec3{}) {
		whiteXYZ = D65White()
	}

	for total := 1; total <= 3*255; total++ {
		for red := 0; red <= 255 && red <= total; red++ {
			for green := 0; green <= 255 && red+green <= total; green++ {
				blue := total - red - green
				if blue < 0 || blue > 255 {
					continue
				}
				rgb8 := [3]uint8{uint8(red), uint8(green), uint8(blue)}
				rgb := Vec3{
					float64(rgb8[0]) / 255,
					float64(rgb8[1]) / 255,
					float64(rgb8[2]) / 255,
				}
				faultyXYZ := faulty.MulVec(rgb)
				repairedXYZ := repaired.MulVec(rgb)
				xyzError := faultyXYZ.Sub(repairedXYZ).Norm()
				deltaE := deltaE76(faultyXYZ, repairedXYZ, whiteXYZ)
				if deltaE > threshold || xyzError > threshold {
					return Counterexample{
						RGB8:        rgb8,
						RGB:         rgb,
						FaultyXYZ:   faultyXYZ,
						RepairedXYZ: repairedXYZ,
						XYZError:    xyzError,
						DeltaE:      deltaE,
					}, true
				}
			}
		}
	}
	return Counterexample{}, false
}

func deltaE76(a, b, whiteXYZ Vec3) float64 {
	labA := xyzToLab(a, whiteXYZ)
	labB := xyzToLab(b, whiteXYZ)
	return labA.Sub(labB).Norm()
}

func xyzToLab(xyz, whiteXYZ Vec3) Vec3 {
	fx := labTransfer(xyz[0] / whiteXYZ[0])
	fy := labTransfer(xyz[1] / whiteXYZ[1])
	fz := labTransfer(xyz[2] / whiteXYZ[2])
	return Vec3{
		116*fy - 16,
		500 * (fx - fy),
		200 * (fy - fz),
	}
}

func labTransfer(value float64) float64 {
	const epsilon = 216.0 / 24389.0
	const kappa = 24389.0 / 27.0
	if value > epsilon {
		return math.Cbrt(value)
	}
	return (kappa*value + 16) / 116
}
