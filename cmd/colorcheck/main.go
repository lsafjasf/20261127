package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"rgbxyz"
)

func main() {
	matrixFlag := flag.String("matrix", "", "9 comma-separated row-major matrix values; defaults to a reflected sRGB matrix")
	originalFlag := flag.String("original", "", "9 values of the intended sign-correct matrix; defaults to a rounded sRGB/D65 matrix")
	referenceFlag := flag.String("reference", "", "9 values of the colorimetric reference matrix; defaults to sRGB/D65")
	whiteFlag := flag.String("white", "", "3 comma-separated D65 XYZ values")
	flag.Parse()

	reference := rgbxyz.SRGBToXYZD65()
	original := rgbxyz.Mat3{
		{0.41246, 0.35758, 0.18044},
		{0.21267, 0.71516, 0.07218},
		{0.01933, 0.11919, 0.95030},
	}
	var err error
	if *originalFlag != "" {
		original, err = parseMatrix(*originalFlag)
		check(err)
	}
	if *referenceFlag != "" {
		reference, err = parseMatrix(*referenceFlag)
		check(err)
	}

	white := rgbxyz.D65White()
	if *whiteFlag != "" {
		white, err = parseVec3(*whiteFlag)
		check(err)
	}

	matrix := original
	if *matrixFlag != "" {
		matrix, err = parseMatrix(*matrixFlag)
		check(err)
	} else {
		matrix, err = rgbxyz.FlipColumn(matrix, 2)
		check(err)
	}

	config := rgbxyz.DefaultConfig()
	config.Reference = &original
	config.WhiteXYZ = white

	initial := rgbxyz.DiagnoseWithConfig(matrix, config)
	printReport("input", initial)

	repaired, final, repairErr := rgbxyz.Repair(matrix, config)
	if repairErr != nil {
		fmt.Fprintf(os.Stderr, "repair failed: %v\n", repairErr)
		os.Exit(1)
	}
	fmt.Println()
	printReport("repaired", final)
	fmt.Printf("recovered-vs-original max entry error: %.3g\n",
		rgbxyz.MaxMatrixDifference(original, repaired))

	originalError := rgbxyz.CompareColors(
		reference, original, rgbxyz.SampleColors(), white,
	)
	repairedError := rgbxyz.CompareColors(
		reference, repaired, rgbxyz.SampleColors(), white,
	)
	fmt.Printf("original color error: max dE76 %.6g, max XYZ %.6g\n",
		originalError.MaxDeltaE, originalError.MaxXYZ)
	fmt.Printf("repaired color error: max dE76 %.6g, max XYZ %.6g (preserved=%t)\n",
		repairedError.MaxDeltaE, repairedError.MaxXYZ,
		rgbxyz.ErrorStatsEqual(originalError, repairedError, 1e-12))

	if counterexample, ok := rgbxyz.FindMinimalCounterexample(
		matrix, repaired, white, 1e-6,
	); ok {
		fmt.Println()
		fmt.Printf("minimal counterexample: %s r=%d g=%d b=%d\n",
			counterexample.Hex(),
			counterexample.RGB8[0], counterexample.RGB8[1], counterexample.RGB8[2])
		fmt.Printf("  linear RGB: %v\n", counterexample.RGB)
		fmt.Printf("  faulty XYZ:   %v\n", counterexample.FaultyXYZ)
		fmt.Printf("  repaired XYZ: %v\n", counterexample.RepairedXYZ)
		fmt.Printf("  dE76: %.6g, XYZ error: %.6g\n",
			counterexample.DeltaE, counterexample.XYZError)
	}
}

func printReport(label string, report rgbxyz.Report) {
	fmt.Printf("[%s]\n", label)
	fmt.Printf("determinant: %.12g\n", report.Determinant)
	fmt.Printf("invertible: %t (round-trip error %.3g)\n",
		report.Invertible, report.InverseError)
	fmt.Printf("handedness: %s\n", report.Handedness)
	fmt.Printf("white point XYZ: %v\n", report.WhitePoint.XYZ)
	fmt.Printf("white point error: %.3g (xy error %.3g)\n",
		report.WhitePoint.MaxError, report.WhitePoint.ChromaticityError)
	for _, probe := range report.Probes {
		fmt.Printf("axis %s: +e -> %v; -e -> %v; polarity=%s\n",
			probe.AxisName,
			probe.PositiveOutput,
			probe.NegativeOutput,
			probe.Polarity,
		)
	}
	fmt.Printf("checks pass: %t\n", report.OK)
	for _, issue := range report.Issues {
		fmt.Printf("  issue: %s\n", issue)
	}
}

func parseMatrix(raw string) (rgbxyz.Mat3, error) {
	values, err := parseFloats(raw, 9)
	if err != nil {
		return rgbxyz.Mat3{}, err
	}
	return rgbxyz.Mat3{
		{values[0], values[1], values[2]},
		{values[3], values[4], values[5]},
		{values[6], values[7], values[8]},
	}, nil
}

func parseVec3(raw string) (rgbxyz.Vec3, error) {
	values, err := parseFloats(raw, 3)
	if err != nil {
		return rgbxyz.Vec3{}, err
	}
	return rgbxyz.Vec3{values[0], values[1], values[2]}, nil
}

func parseFloats(raw string, expected int) ([]float64, error) {
	fields := strings.Split(raw, ",")
	if len(fields) != expected {
		return nil, fmt.Errorf("expected %d comma-separated values, got %d", expected, len(fields))
	}
	values := make([]float64, expected)
	for i, field := range fields {
		value, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err != nil {
			return nil, fmt.Errorf("value %d: %w", i, err)
		}
		values[i] = value
	}
	return values, nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
