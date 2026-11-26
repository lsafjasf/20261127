package multicam

import (
	"fmt"
	"sort"
)

type alignmentPoint struct {
	masterSeconds float64
	localSeconds  float64
	errorSeconds  float64
}

type machineAlignment struct {
	status  string
	reason  string
	offset  float64
	report  AlignmentReport
	hasData bool
}

// calculateAlignments uses only observations belonging to the target machine.
// It deliberately has no group/team offset fallback: a machine that cannot
// prove its own alignment is rejected rather than borrowing another offset.
func calculateAlignments(project Project) map[string]machineAlignment {
	pointsByMachine := make(map[string][]alignmentPoint, len(project.Machines))
	for _, clap := range project.Claps {
		for _, observation := range clap.Observations {
			pointsByMachine[observation.MachineID] = append(
				pointsByMachine[observation.MachineID],
				alignmentPoint{
					masterSeconds: clap.MasterSeconds,
					localSeconds:  observation.LocalSeconds,
					errorSeconds:  observation.ErrorSeconds,
				},
			)
		}
	}

	alignments := make(map[string]machineAlignment, len(project.Machines))
	for _, machine := range project.Machines {
		points := pointsByMachine[machine.ID]
		if len(points) == 0 {
			alignments[machine.ID] = machineAlignment{
				status: StatusMissingClap,
				reason: "no clap observation for this machine; no offset was applied",
			}
			continue
		}

		offsets := make([]float64, 0, len(points))
		maxObservationError := 0.0
		for _, point := range points {
			offsets = append(offsets, point.masterSeconds-point.localSeconds)
			if point.errorSeconds > maxObservationError {
				maxObservationError = point.errorSeconds
			}
		}
		sort.Float64s(offsets)
		offset := medianSorted(offsets)

		maxResidual := 0.0
		for _, point := range points {
			residual := point.masterSeconds - (point.localSeconds + offset)
			if residual < 0 {
				residual = -residual
			}
			if residual > maxResidual {
				maxResidual = residual
			}
		}

		maxError := maxObservationError
		if maxResidual > maxError {
			maxError = maxResidual
		}
		report := AlignmentReport{
			OffsetSeconds:              offset,
			ObservationCount:           len(points),
			MaxObservationErrorSeconds: maxObservationError,
			MaxResidualSeconds:         maxResidual,
			MaxErrorSeconds:            maxError,
			ToleranceSeconds:           project.Sequence.ToleranceSeconds,
		}

		alignment := machineAlignment{
			status:  StatusAligned,
			offset:  offset,
			report:  report,
			hasData: true,
		}
		if maxError > project.Sequence.ToleranceSeconds {
			alignment.status = StatusToleranceExceeded
			alignment.reason = fmt.Sprintf(
				"alignment error %.6gs exceeds tolerance %.6gs; team offset was not applied",
				maxError,
				project.Sequence.ToleranceSeconds,
			)
		}
		alignments[machine.ID] = alignment
	}
	return alignments
}

func medianSorted(values []float64) float64 {
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}
