package rgbxyz

import (
	"fmt"
	"math"
	"strings"
)

// Repair reflects the columns identified by the axis probes, then re-runs all
// checks. It never silently changes a matrix that already passes validation.
func Repair(matrix Mat3, config Config) (Mat3, Report, error) {
	config = config.withDefaults()
	initial := DiagnoseWithConfig(matrix, config)
	if initial.OK {
		return matrix, initial, nil
	}

	axes := append([]int(nil), initial.FlippedAxes...)
	if len(axes) == 0 {
		axis, ok := locateByWhitePoint(matrix, config)
		if !ok {
			return matrix, initial, repairError("could not locate a single reflected RGB axis", initial)
		}
		axes = []int{axis}
	}

	repaired := matrix
	for _, axis := range axes {
		var err error
		repaired, err = FlipColumn(repaired, axis)
		if err != nil {
			return matrix, initial, err
		}
	}

	final := DiagnoseWithConfig(repaired, config)
	if !final.OK {
		return matrix, initial, repairError("matrix still fails validation after repair", final)
	}
	return repaired, final, nil
}

func locateByWhitePoint(matrix Mat3, config Config) (int, bool) {
	bestAxis := -1
	bestError := math.Inf(1)
	for axis := 0; axis < 3; axis++ {
		candidate, err := FlipColumn(matrix, axis)
		if err != nil {
			continue
		}
		if !candidate.InvertibleFor(config.RelativeTolerance) {
			continue
		}
		if candidate.Det() <= 0 {
			continue
		}
		whiteError := checkWhitePoint(candidate, config).MaxError
		if whiteError < bestError {
			bestAxis = axis
			bestError = whiteError
		}
	}
	if bestAxis < 0 || bestError > config.WhiteTolerance {
		return -1, false
	}
	return bestAxis, true
}

func repairError(message string, report Report) error {
	if len(report.Issues) == 0 {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("%s: %s", message, strings.Join(report.Issues, "; "))
}
