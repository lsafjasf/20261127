package rgbxyz

import (
	"fmt"
	"math"
)

// Handedness describes the orientation of a basis mapping.
type Handedness int

const (
	// HandednessDegenerate means the determinant is indistinguishable from zero.
	HandednessDegenerate Handedness = iota
	// HandednessRight means the determinant is positive.
	HandednessRight
	// HandednessLeft means the determinant is negative.
	HandednessLeft
)

func (h Handedness) String() string {
	switch h {
	case HandednessRight:
		return "right-handed"
	case HandednessLeft:
		return "left-handed"
	default:
		return "degenerate"
	}
}

// Polarity describes how a probed axis compares with the reference matrix.
type Polarity int

const (
	PolarityUnknown Polarity = iota
	PolarityAligned
	PolarityFlipped
)

func (p Polarity) String() string {
	switch p {
	case PolarityAligned:
		return "aligned"
	case PolarityFlipped:
		return "flipped"
	default:
		return "unknown"
	}
}

// AxisProbe records the result of applying the matrix to both signs of one
// input basis vector.
type AxisProbe struct {
	Axis            int
	AxisName        string
	PositiveInput   Vec3
	NegativeInput   Vec3
	PositiveOutput  Vec3
	NegativeOutput  Vec3
	AntisymmetryErr float64
	Polarity        Polarity
	PositiveGap     float64
	NegativeGap     float64
}

// WhitePointReport records the measured and expected XYZ white point.
type WhitePointReport struct {
	RGB                  Vec3
	XYZ                  Vec3
	ExpectedXYZ          Vec3
	ErrorXYZ             Vec3
	MaxError             float64
	Chromaticity         [2]float64
	ExpectedChromaticity [2]float64
	ChromaticityError    float64
	OK                   bool
}

// Report is the result of validating one RGB-to-XYZ matrix.
type Report struct {
	Matrix       Mat3
	Determinant  float64
	Invertible   bool
	Inverse      Mat3
	InverseError float64
	Handedness   Handedness
	WhitePoint   WhitePointReport
	Probes       []AxisProbe
	FlippedAxes  []int
	Issues       []string
	OK           bool
}

// Config controls matrix validation. A nil Reference disables reference-axis
// comparison; handedness and white-point checks still run.
type Config struct {
	Reference         *Mat3
	WhiteRGB          Vec3
	WhiteXYZ          Vec3
	RelativeTolerance float64
	WhiteTolerance    float64
	ProbeTolerance    float64
	InverseTolerance  float64
}

// DefaultConfig returns the sRGB/D65 defaults used by the package example.
func DefaultConfig() Config {
	reference := SRGBToXYZD65()
	return Config{
		Reference:         &reference,
		WhiteRGB:          RGBWhite(),
		WhiteXYZ:          D65White(),
		RelativeTolerance: RelativeTolerance,
		WhiteTolerance:    2e-5,
		ProbeTolerance:    1e-7,
		InverseTolerance:  1e-8,
	}
}

func (c Config) withDefaults() Config {
	if c.WhiteRGB == (Vec3{}) {
		c.WhiteRGB = RGBWhite()
	}
	if c.WhiteXYZ == (Vec3{}) {
		c.WhiteXYZ = D65White()
	}
	if c.RelativeTolerance <= 0 {
		c.RelativeTolerance = RelativeTolerance
	}
	if c.WhiteTolerance <= 0 {
		c.WhiteTolerance = 2e-5
	}
	if c.ProbeTolerance <= 0 {
		c.ProbeTolerance = 1e-7
	}
	if c.InverseTolerance <= 0 {
		c.InverseTolerance = 1e-8
	}
	return c
}

var axisNames = [3]string{"R", "G", "B"}

// Diagnose validates a matrix using the package's sRGB/D65 defaults.
func Diagnose(matrix Mat3) Report {
	return DiagnoseWithConfig(matrix, DefaultConfig())
}

// DiagnoseWithConfig validates invertibility, handedness, white point, and
// per-axis polarity.
func DiagnoseWithConfig(matrix Mat3, config Config) Report {
	config = config.withDefaults()
	report := Report{Matrix: matrix, Determinant: matrix.Det()}

	if !matrix.IsFinite() || math.IsNaN(report.Determinant) || math.IsInf(report.Determinant, 0) {
		report.Issues = append(report.Issues, "matrix contains non-finite values")
		report.Handedness = HandednessDegenerate
		return finishReport(report)
	}

	report.Invertible = matrix.InvertibleFor(config.RelativeTolerance)
	if report.Invertible {
		inverse, err := matrix.InverseFor(config.RelativeTolerance)
		if err != nil {
			report.Invertible = false
			report.Issues = append(report.Issues, "matrix inverse failed: "+err.Error())
		} else {
			report.Inverse = inverse
			report.InverseError = RoundTripError(matrix, inverse)
			if report.InverseError > config.InverseTolerance {
				report.Invertible = false
				report.Issues = append(report.Issues, fmt.Sprintf(
					"inverse round-trip error %.3g exceeds %.3g",
					report.InverseError, config.InverseTolerance,
				))
			}
		}
	} else {
		report.Issues = append(report.Issues, "matrix is singular or numerically unstable")
	}

	detTolerance := matrix.DeterminantToleranceFor(config.RelativeTolerance)
	switch {
	case report.Determinant > detTolerance:
		report.Handedness = HandednessRight
	case report.Determinant < -detTolerance:
		report.Handedness = HandednessLeft
		report.Issues = append(report.Issues, "negative determinant: basis orientation is reversed")
	default:
		report.Handedness = HandednessDegenerate
	}

	report.WhitePoint = checkWhitePoint(matrix, config)
	if !report.WhitePoint.OK {
		report.Issues = append(report.Issues, fmt.Sprintf(
			"white point XYZ error %.3g exceeds %.3g",
			report.WhitePoint.MaxError, config.WhiteTolerance,
		))
	}

	report.Probes = probeAxes(matrix, config)
	for _, probe := range report.Probes {
		if probe.Polarity == PolarityFlipped {
			report.FlippedAxes = append(report.FlippedAxes, probe.Axis)
		}
	}
	if len(report.FlippedAxes) > 0 {
		report.Issues = append(report.Issues, fmt.Sprintf(
			"reference axis polarity flipped: %s",
			axisList(report.FlippedAxes),
		))
	}

	return finishReport(report)
}

func finishReport(report Report) Report {
	report.OK = report.Invertible &&
		report.Handedness == HandednessRight &&
		report.WhitePoint.OK &&
		len(report.FlippedAxes) == 0
	return report
}

func checkWhitePoint(matrix Mat3, config Config) WhitePointReport {
	actual := matrix.MulVec(config.WhiteRGB)
	errorXYZ := actual.Sub(config.WhiteXYZ)
	actualChromaticity := chromaticity(actual)
	expectedChromaticity := chromaticity(config.WhiteXYZ)
	chromaticityError := math.Hypot(
		actualChromaticity[0]-expectedChromaticity[0],
		actualChromaticity[1]-expectedChromaticity[1],
	)
	maximumError := MaxDifference(actual, config.WhiteXYZ)
	return WhitePointReport{
		RGB:                  config.WhiteRGB,
		XYZ:                  actual,
		ExpectedXYZ:          config.WhiteXYZ,
		ErrorXYZ:             errorXYZ,
		MaxError:             maximumError,
		Chromaticity:         actualChromaticity,
		ExpectedChromaticity: expectedChromaticity,
		ChromaticityError:    chromaticityError,
		OK:                   maximumError <= config.WhiteTolerance,
	}
}

func chromaticity(v Vec3) [2]float64 {
	sum := v[0] + v[1] + v[2]
	if math.Abs(sum) < 1e-15 {
		return [2]float64{}
	}
	return [2]float64{v[0] / sum, v[1] / sum}
}

func probeAxes(matrix Mat3, config Config) []AxisProbe {
	probes := make([]AxisProbe, 0, 3)
	for axis := 0; axis < 3; axis++ {
		input := Vec3{}
		input[axis] = 1
		probe := AxisProbe{
			Axis:           axis,
			AxisName:       axisNames[axis],
			PositiveInput:  input,
			NegativeInput:  input.Scale(-1),
			PositiveOutput: matrix.MulVec(input),
			NegativeOutput: matrix.MulVec(input.Scale(-1)),
		}
		probe.AntisymmetryErr = MaxDifference(
			probe.PositiveOutput,
			probe.NegativeOutput.Scale(-1),
		)

		if config.Reference != nil {
			expectedPositive := config.Reference.MulVec(input)
			expectedNegative := expectedPositive.Scale(-1)
			probe.PositiveGap = MaxDifference(probe.PositiveOutput, expectedPositive)
			probe.NegativeGap = MaxDifference(probe.NegativeOutput, expectedNegative)
			flippedGap := MaxDifference(probe.PositiveOutput, expectedNegative)
			tolerance := config.ProbeTolerance * math.Max(1, expectedPositive.Norm())
			aligned := probe.PositiveGap <= tolerance &&
				MaxDifference(probe.NegativeOutput, expectedNegative) <= tolerance
			flipped := flippedGap <= tolerance &&
				MaxDifference(probe.NegativeOutput, expectedPositive) <= tolerance
			switch {
			case aligned:
				probe.Polarity = PolarityAligned
			case flipped:
				probe.Polarity = PolarityFlipped
			}
		}
		probes = append(probes, probe)
	}
	return probes
}

func axisList(axes []int) string {
	names := make([]string, len(axes))
	for i, axis := range axes {
		if axis >= 0 && axis < len(axisNames) {
			names[i] = axisNames[axis]
		} else {
			names[i] = fmt.Sprintf("axis-%d", axis)
		}
	}
	return fmt.Sprint(names)
}
