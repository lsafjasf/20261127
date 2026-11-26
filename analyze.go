package tailtrace

import (
	"fmt"
)

const defaultToleranceNS = 1_000_000 // 1 ms

func requireRational(r Rational, name string) (Rational, error) {
	out, err := NewRational(r.Num, r.Den)
	if err != nil {
		return Rational{}, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

func optionalRational(r *Rational, name string) (*Rational, error) {
	if r == nil {
		return nil, nil
	}
	out, err := requireRational(*r, name)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func resolveFramePTS(frame Frame, defaultTimeBase *Rational) (Rational, error) {
	if frame.PTSTicks != nil {
		timeBase := defaultTimeBase
		if frame.TimeBase != nil {
			tb, err := requireRational(*frame.TimeBase, "frame.time_base")
			if err != nil {
				return Rational{}, err
			}
			timeBase = &tb
		}
		if timeBase == nil {
			return Rational{}, fmt.Errorf("frame %d has pts_ticks but no time_base", frame.Index)
		}
		tb, err := requireRational(*timeBase, "time_base")
		if err != nil {
			return Rational{}, err
		}
		return MustRational(*frame.PTSTicks, 1).Mul(tb), nil
	}
	if frame.PTS == nil {
		return Rational{}, fmt.Errorf("frame %d requires pts or pts_ticks", frame.Index)
	}
	return requireRational(*frame.PTS, "frame.pts")
}

// Analyze reconciles output PTS, delivery contract duration, and player
// display intervals. Schema-level problems return an error; media-level
// mismatches are represented in Report.Findings.
func Analyze(evidence Evidence) (*Report, error) {
	fps, err := requireRational(evidence.Contract.FPS, "contract.fps")
	if err != nil {
		return nil, err
	}
	if fps.Num <= 0 || fps.Den <= 0 {
		return nil, fmt.Errorf("contract.fps must be positive, got %s", fps)
	}
	frameDuration := Rational{Num: fps.Den, Den: fps.Num}
	startPTS, err := requireRational(evidence.Contract.StartPTS, "contract.start_pts")
	if err != nil {
		return nil, err
	}
	tolerance := MustRational(defaultToleranceNS, 1_000_000_000)
	if evidence.Contract.Tolerance != nil {
		tolerance, err = requireRational(*evidence.Contract.Tolerance, "contract.time_tolerance")
		if err != nil {
			return nil, err
		}
	}

	output := evidence.Output
	if len(output) == 0 {
		return nil, fmt.Errorf("output must contain at least one frame")
	}
	contentFrames := evidence.Contract.ContentFrames
	if contentFrames == 0 {
		contentFrames = int64(len(evidence.Source))
	}
	if contentFrames < 0 || contentFrames > int64(len(output)) {
		return nil, fmt.Errorf("content_frames=%d is invalid for output_frames=%d", contentFrames, len(output))
	}

	playerByIndex := make(map[int64]PlayerFrame, len(evidence.Contract.Player.Frames))
	for _, observed := range evidence.Contract.Player.Frames {
		if _, exists := playerByIndex[observed.Index]; exists {
			return nil, fmt.Errorf("duplicate player evidence for frame index %d", observed.Index)
		}
		playerByIndex[observed.Index] = observed
	}

	report := &Report{OK: true}
	findings := make([]Finding, 0, 8)
	traces := make([]FrameTrace, 0, len(output))
	nonCFR := 0
	indexGaps := 0
	unknownExtra := int64(0)
	tailBlack := int64(0)
	var firstExtraIndex int64 = -1
	var firstExtraPTS *Rational

	for position, frame := range output {
		pts, err := resolveFramePTS(frame, evidence.PTSTimeBase)
		if err != nil {
			return nil, err
		}
		expectedPTS := startPTS.Add(MustRational(int64(position), 1).Mul(frameDuration))
		drift := pts.Sub(expectedPTS)
		if drift.Cmp(ZeroRational()) != 0 {
			nonCFR++
		}
		if frame.Index != int64(position) {
			indexGaps++
		}

		role := RoleContent
		if int64(position) >= contentFrames {
			if firstExtraIndex < 0 {
				firstExtraIndex = int64(position)
				firstExtraPTS = &pts
			}
			if frame.Black || frame.Origin == "tail_pad" {
				role = RoleTailBlackPad
				tailBlack++
			} else {
				role = RoleExtraUnknown
				unknownExtra++
			}
		}

		displayEnd := pts.Add(frameDuration)
		if position+1 < len(output) {
			nextPTS, err := resolveFramePTS(output[position+1], evidence.PTSTimeBase)
			if err != nil {
				return nil, err
			}
			displayEnd = nextPTS
		}

		trace := FrameTrace{
			Index: frame.Index, Role: role, Origin: frame.Origin, Black: frame.Black,
			PTS: pts, ExpectedPTS: expectedPTS, PTSDrift: drift,
			DisplayStart: pts, DisplayEnd: displayEnd,
			PTSTicks: frame.PTSTicks, TimeBase: frame.TimeBase, SourceIndex: frame.SourceIndex,
		}
		if observed, ok := playerByIndex[frame.Index]; ok {
			presented := observed.Presented
			trace.PlayerPresented = &presented
			if observed.ShownStart != nil {
				shownStart, err := optionalRational(observed.ShownStart, "player.shown_start")
				if err != nil {
					return nil, err
				}
				delta := shownStart.Sub(pts)
				if absRat(delta).Cmp(tolerance) > 0 {
					findings = append(findings, Finding{
						Code: "player_start_mismatch", Severity: SeverityWarning,
						Message: fmt.Sprintf("frame %d shown at %s but PTS is %s", frame.Index, shownStart.SecondsText(6), pts.SecondsText(6)),
					})
				}
			}
			if observed.ShownEnd != nil {
				shownEnd, err := optionalRational(observed.ShownEnd, "player.shown_end")
				if err != nil {
					return nil, err
				}
				delta := shownEnd.Sub(displayEnd)
				if absRat(delta).Cmp(tolerance) > 0 {
					findings = append(findings, Finding{
						Code: "player_end_mismatch", Severity: SeverityWarning,
						Message: fmt.Sprintf("frame %d held until %s but next-PTS/end is %s", frame.Index, shownEnd.SecondsText(6), displayEnd.SecondsText(6)),
					})
				}
			}
		}
		traces = append(traces, trace)
	}

	if nonCFR > 0 {
		report.OK = false
		findings = append(findings, Finding{
			Code: "pts_not_cfr", Severity: SeverityError,
			Message: fmt.Sprintf("%d output PTS values differ from start + index*frame_duration; inspect timestamp generation before blaming duration metadata", nonCFR),
		})
	}
	if indexGaps > 0 {
		findings = append(findings, Finding{
			Code: "frame_index_discontinuity", Severity: SeverityWarning,
			Message: fmt.Sprintf("%d frame indexes are not contiguous from zero", indexGaps),
		})
	}

	return finishAnalysis(evidence, report, traces, findings, analysisCounters{
		contentFrames: contentFrames, tailBlack: tailBlack, unknownExtra: unknownExtra,
		firstExtraIndex: firstExtraIndex, firstExtraPTS: firstExtraPTS,
		frameDuration: frameDuration, tolerance: tolerance,
	})
}

type analysisCounters struct {
	contentFrames   int64
	tailBlack       int64
	unknownExtra    int64
	firstExtraIndex int64
	firstExtraPTS   *Rational
	frameDuration   Rational
	tolerance       Rational
}

func absRat(r Rational) Rational {
	if r.Num < 0 {
		return MustRational(-r.Num, r.Den)
	}
	return r
}
