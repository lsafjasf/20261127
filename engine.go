package multicam

// Analyze validates the project, rejects unproven alignments, and returns the
// four-layer mapping together with each machine's master-timeline coverage.
func Analyze(project Project) (Report, error) {
	if err := project.Validate(); err != nil {
		return Report{}, err
	}

	lensesByMachine := make(map[string][]Lens, len(project.Machines))
	for _, lens := range project.Lenses {
		lensesByMachine[lens.MachineID] = append(lensesByMachine[lens.MachineID], lens)
	}
	clipsByMachine := make(map[string][]Clip, len(project.Machines))
	for _, clip := range project.Clips {
		clipsByMachine[clip.MachineID] = append(clipsByMachine[clip.MachineID], clip)
	}
	alignments := calculateAlignments(project)

	report := Report{
		Sequence: ReportSequence{
			ID:                 project.Sequence.ID,
			Name:               project.Sequence.Name,
			MasterStartSeconds: 0,
			MasterEndSeconds:   project.Sequence.MasterDurationSeconds,
		},
		ToleranceSeconds: project.Sequence.ToleranceSeconds,
		Channels:         make([]ChannelReport, 0, len(project.Machines)),
	}

	for _, machine := range project.Machines {
		channel := newChannelReport(machine, lensesByMachine[machine.ID])
		alignment := alignments[machine.ID]
		if alignment.hasData {
			alignmentCopy := alignment.report
			channel.Alignment = &alignmentCopy
		}

		machineClips := clipsByMachine[machine.ID]
		if len(machineClips) == 0 {
			channel.Status = StatusNoClips
			channel.RejectionReason = "machine has no clips in this sequence"
			report.Channels = append(report.Channels, channel)
			continue
		}

		channel.Status = alignment.status
		channel.RejectionReason = alignment.reason
		if alignment.status != StatusAligned {
			report.Channels = append(report.Channels, channel)
			continue
		}

		if channel.Alignment != nil {
			channel.Alignment.Applied = true
		}
		applyMachineClips(&channel, machineClips, alignment.offset, project.Sequence.MasterDurationSeconds)
		report.Channels = append(report.Channels, channel)
	}
	return report, nil
}

func newChannelReport(machine Machine, lenses []Lens) ChannelReport {
	channel := ChannelReport{
		Machine:  machine,
		Coverage: []Interval{},
		Lenses:   make([]LensReport, 0, len(lenses)),
	}
	for _, lens := range lenses {
		channel.Lenses = append(channel.Lenses, LensReport{
			Lens:     lens,
			Clips:    []ClipReport{},
			Coverage: []Interval{},
		})
	}
	return channel
}

func applyMachineClips(
	channel *ChannelReport,
	clips []Clip,
	offset float64,
	masterDuration float64,
) {
	lensPositions := make(map[string]int, len(channel.Lenses))
	for i, lensReport := range channel.Lenses {
		lensPositions[lensReport.Lens.ID] = i
	}

	coverage := make([]Interval, 0, len(clips))
	for _, clip := range clips {
		local := Interval{StartSeconds: clip.StartSeconds, EndSeconds: clip.EndSeconds}
		clipReport := ClipReport{ID: clip.ID, LocalInterval: local}
		master, clipped, covered := projectInterval(local, offset, masterDuration)
		clipReport.MasterInterval = &master

		lensPosition := lensPositions[clip.LensID]
		lensReport := &channel.Lenses[lensPosition]
		if covered {
			clipReport.CoverageInterval = &clipped
			coverage = append(coverage, clipped)
			lensReport.Coverage = append(lensReport.Coverage, clipped)
		}
		lensReport.Clips = append(lensReport.Clips, clipReport)
	}

	channel.Coverage = mergeIntervals(coverage)
	channel.CoveredSeconds = coveredSeconds(channel.Coverage)
	for i := range channel.Lenses {
		channel.Lenses[i].Coverage = mergeIntervals(channel.Lenses[i].Coverage)
		channel.Lenses[i].CoveredSeconds = coveredSeconds(channel.Lenses[i].Coverage)
	}
}
