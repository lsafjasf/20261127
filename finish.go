package tailtrace

import "fmt"

func ratPtr(r Rational) *Rational { return &r }

func finishAnalysis(evidence Evidence, report *Report, traces []FrameTrace, findings []Finding, counters analysisCounters) (*Report, error) {
	frameDuration := counters.frameDuration
	outputFrames := int64(len(traces))
	last := traces[outputFrames-1]
	cfrDuration := MustRational(outputFrames, 1).Mul(frameDuration)

	firstBlackPTS := (*Rational)(nil)
	for i := range traces {
		if traces[i].Role == RoleTailBlackPad {
			firstBlackPTS = &traces[i].PTS
			break
		}
	}
	tailBlackDuration := MustRational(counters.tailBlack, 1).Mul(frameDuration)

	contractFrames := counters.contentFrames
	contractDuration := MustRational(counters.contentFrames, 1).Mul(frameDuration)
	if evidence.Contract.DeliveryFrames != nil {
		contractFrames = *evidence.Contract.DeliveryFrames
		if contractFrames < 0 {
			return nil, fmt.Errorf("delivery_frames must be non-negative")
		}
		contractDuration = MustRational(contractFrames, 1).Mul(frameDuration)
	} else if evidence.Contract.DeliveryDuration != nil {
		duration, err := optionalRational(evidence.Contract.DeliveryDuration, "contract.delivery_duration")
		if err != nil {
			return nil, err
		}
		frameCount := duration.Div(frameDuration)
		if frameCount.Den != 1 {
			return nil, fmt.Errorf("delivery_duration %s is not an integer number of CFR frames (%s)", duration, frameDuration)
		}
		contractFrames = frameCount.Num
		contractDuration = *duration
	}

	durations := Durations{
		FPS: evidence.Contract.FPS.normalize(), FrameDuration: frameDuration,
		ContentFrames:   counters.contentFrames,
		ContentDuration: MustRational(counters.contentFrames, 1).Mul(frameDuration),
		OutputFrames:    outputFrames, LastPTS: last.PTS, LastPTSEnd: last.DisplayEnd,
		CFRDuration: cfrDuration, TailBlackFrames: counters.tailBlack,
		TailBlackStartPTS: firstBlackPTS, TailBlackDuration: tailBlackDuration,
		ContractFrames: contractFrames, ContractDuration: contractDuration,
		PlayerScheduleEnd: last.DisplayEnd,
	}

	if evidence.Contract.ContainerDuration != nil {
		containerDuration, err := optionalRational(evidence.Contract.ContainerDuration, "contract.container_duration")
		if err != nil {
			return nil, err
		}
		durations.ContainerDuration = containerDuration
	}
	if evidence.Contract.AudioDuration != nil {
		audioDuration, err := optionalRational(evidence.Contract.AudioDuration, "contract.audio_duration")
		if err != nil {
			return nil, err
		}
		durations.AudioDuration = audioDuration
	}
	if evidence.Contract.Player.ReportedDuration != nil {
		reported, err := optionalRational(evidence.Contract.Player.ReportedDuration, "player.reported_duration")
		if err != nil {
			return nil, err
		}
		durations.PlayerReportedEnd = reported
	}
	if evidence.Contract.Player.EditListEnd != nil {
		editEnd, err := optionalRational(evidence.Contract.Player.EditListEnd, "player.edit_list_end")
		if err != nil {
			return nil, err
		}
		durations.EditListEnd = editEnd
	}

	extraFrames := outputFrames - counters.contentFrames
	if extraFrames > 0 && counters.unknownExtra == 0 {
		expectedEnd := durations.ContentDuration
		findings = append(findings, Finding{
			Code: "tail_black_padding_located", Severity: SeverityInfo,
			Message: fmt.Sprintf(
				"frames %d..%d are CFR-contiguous black tail padding generated after the %d content frames; first PTS=%s, displayed for %s",
				counters.firstExtraIndex, outputFrames-1, counters.contentFrames,
				counters.firstExtraPTS.SecondsText(6), tailBlackDuration.SecondsText(6),
			),
			ExtraFrames: counters.tailBlack, FirstExtraIndex: counters.firstExtraIndex,
			FirstExtraPTS: counters.firstExtraPTS, ExtraDuration: ratPtr(tailBlackDuration),
			ExpectedDuration: &expectedEnd, ActualDuration: &last.DisplayEnd,
		})
	}
	if counters.unknownExtra > 0 {
		report.OK = false
		findings = append(findings, Finding{
			Code: "unexplained_extra_frames", Severity: SeverityError,
			Message:     fmt.Sprintf("%d extra frames after content boundary are neither black nor marked origin=tail_pad; inspect renderer/source mapping", counters.unknownExtra),
			ExtraFrames: counters.unknownExtra, FirstExtraIndex: counters.firstExtraIndex,
			FirstExtraPTS: counters.firstExtraPTS,
		})
	}
	if evidence.Contract.DeliveryFrames != nil && *evidence.Contract.DeliveryFrames != outputFrames {
		report.OK = false
		expected := contractDuration
		actual := cfrDuration
		findings = append(findings, Finding{
			Code: "contract_frame_count_mismatch", Severity: SeverityError,
			Message:     fmt.Sprintf("contract delivery_frames=%d but output contains %d frames", contractFrames, outputFrames),
			ExtraFrames: outputFrames - contractFrames, ExpectedDuration: &expected, ActualDuration: &actual,
		})
	}
	if evidence.Contract.DeliveryFrames != nil && evidence.Contract.DeliveryDuration != nil {
		if evidence.Contract.DeliveryDuration.Cmp(contractDuration) != 0 {
			report.OK = false
			deliveryDuration := evidence.Contract.DeliveryDuration.normalize()
			findings = append(findings, Finding{
				Code: "contract_duration_not_frame_aligned", Severity: SeverityError,
				Message: fmt.Sprintf("delivery_duration=%s disagrees with delivery_frames*frame_duration=%s", deliveryDuration.SecondsText(6), contractDuration.SecondsText(6)),
			})
		}
	}
	if counters.tailBlack > evidence.Contract.AllowedTailBlackFrames {
		report.OK = false
		findings = append(findings, Finding{
			Code: "tail_black_outside_contract", Severity: SeverityError,
			Message:     fmt.Sprintf("output has %d tail-black frames but contract allows %d; trim renderer tail padding or revise the contract", counters.tailBlack, evidence.Contract.AllowedTailBlackFrames),
			ExtraFrames: counters.tailBlack - evidence.Contract.AllowedTailBlackFrames,
		})
	} else if counters.tailBlack > 0 {
		findings = append(findings, Finding{
			Code: "tail_black_contract_permitted", Severity: SeverityInfo,
			Message: fmt.Sprintf("contract allows %d tail-black frame(s); distinguish content duration %s from delivery/container duration %s", evidence.Contract.AllowedTailBlackFrames, durations.ContentDuration.SecondsText(6), cfrDuration.SecondsText(6)),
		})
	}

	effectivePlayerEnd := last.DisplayEnd
	if durations.EditListEnd != nil {
		effectivePlayerEnd = *durations.EditListEnd
		if durations.EditListEnd.Cmp(last.DisplayEnd) < 0 {
			findings = append(findings, Finding{
				Code: "edit_list_trims_tail_for_players", Severity: SeverityInfo,
				Message: fmt.Sprintf("edit list ends at %s; edit-list-aware players stop before sample end %s, although extra samples remain in the container", durations.EditListEnd.SecondsText(6), last.DisplayEnd.SecondsText(6)),
			})
		}
	}
	if durations.ContainerDuration != nil {
		actual := *durations.ContainerDuration
		if actual.Cmp(last.DisplayEnd) != 0 {
			report.OK = false
			expected := last.DisplayEnd
			findings = append(findings, Finding{
				Code: "container_duration_mismatch", Severity: SeverityError,
				Message:          fmt.Sprintf("container/metadata duration is %s but CFR sample end is %s; this is a muxer duration issue, not the source of black frames", actual.SecondsText(6), expected.SecondsText(6)),
				ExpectedDuration: &expected, ActualDuration: &actual,
			})
		} else if durations.EditListEnd != nil {
			findings = append(findings, Finding{
				Code: "container_duration_differs_from_edit_list", Severity: SeverityInfo,
				Message: fmt.Sprintf("container/sample duration is %s while edit-list playback ends at %s; this is valid when an edit list trims the tail", actual.SecondsText(6), effectivePlayerEnd.SecondsText(6)),
			})
		}
	}
	if durations.AudioDuration != nil {
		audioDelta := absRat(durations.AudioDuration.Sub(cfrDuration))
		switch {
		case counters.tailBlack > 0 && audioDelta.Cmp(counters.tolerance) <= 0:
			findings = append(findings, Finding{
				Code: "tail_padding_aligns_with_audio_duration", Severity: SeverityInfo,
				Message: fmt.Sprintf("padded video end matches audio duration %s; renderer may be extending video to the audio/master timeline rather than changing PTS", durations.AudioDuration.SecondsText(6)),
			})
		case audioDelta.Cmp(counters.tolerance) > 0:
			findings = append(findings, Finding{
				Code: "audio_video_duration_mismatch", Severity: SeverityWarning,
				Message:          fmt.Sprintf("audio duration %s differs from CFR video end %s; verify whether tail padding or truncation is the intended A/V alignment", durations.AudioDuration.SecondsText(6), cfrDuration.SecondsText(6)),
				ExpectedDuration: &cfrDuration, ActualDuration: durations.AudioDuration,
			})
		}
	}
	if durations.PlayerReportedEnd != nil {
		delta := absRat(durations.PlayerReportedEnd.Sub(effectivePlayerEnd))
		if delta.Cmp(counters.tolerance) > 0 {
			findings = append(findings, Finding{
				Code: "player_duration_mismatch", Severity: SeverityWarning,
				Message:          fmt.Sprintf("player reports duration %s but expected display end is %s within tolerance", durations.PlayerReportedEnd.SecondsText(6), effectivePlayerEnd.SecondsText(6)),
				ExpectedDuration: &effectivePlayerEnd, ActualDuration: durations.PlayerReportedEnd,
			})
		}
	}
	for _, trace := range traces {
		if trace.PlayerPresented != nil && !*trace.PlayerPresented {
			findings = append(findings, Finding{
				Code: "player_did_not_present_frame", Severity: SeverityWarning,
				Message: fmt.Sprintf("player evidence says output frame %d was not presented", trace.Index),
			})
		}
	}

	report.Frames = traces
	report.Durations = durations
	report.Findings = findings
	classifyReport(report, counters, evidence)
	return report, nil
}

func classifyReport(report *Report, counters analysisCounters, evidence Evidence) {
	d := report.Durations
	contractAllows := counters.tailBlack <= evidence.Contract.AllowedTailBlackFrames
	if evidence.Contract.DeliveryFrames != nil {
		contractAllows = contractAllows && *evidence.Contract.DeliveryFrames == d.OutputFrames
	}

	switch {
	case hasFinding(report.Findings, "pts_not_cfr"):
		report.Classification = "pts_generation_error"
		report.Summary = "PTS is not CFR-contiguous; the extra display time cannot yet be attributed to intentional tail padding."
	case counters.unknownExtra > 0:
		report.Classification = "unexplained_extra_frames"
		report.Summary = "Extra output frames are present after the content boundary, but they are not identified as black renderer padding."
	case counters.tailBlack > 0 && contractAllows && report.OK:
		report.Classification = "contract_allowed_tail_black_padding"
		report.Summary = fmt.Sprintf(
			"Renderer appended %d CFR-contiguous black frame(s), and the output contract allows them. Content duration is %s; delivery/player duration including black is %s.",
			counters.tailBlack, d.ContentDuration.SecondsText(6), d.CFRDuration.SecondsText(6),
		)
	case counters.tailBlack > 0:
		report.Classification = "renderer_tail_black_padding_contract_violation"
		report.Summary = fmt.Sprintf(
			"The two visible extra frames originate in renderer tail-black padding (indices %d and %d), not in PTS rounding or player timing. They add %s; the contract duration is %s. Trim the padding or explicitly contract for %d lead-out frame(s).",
			counters.contentFrames, counters.contentFrames+1, d.TailBlackDuration.SecondsText(6), d.ContractDuration.SecondsText(6), counters.tailBlack,
		)
	default:
		report.Classification = "clean_cfr_output"
		report.Summary = fmt.Sprintf("No tail-black extras found. %d CFR frames end at %s.", d.OutputFrames, d.CFRDuration.SecondsText(6))
	}
}

func hasFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
