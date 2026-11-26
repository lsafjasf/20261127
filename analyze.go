package hdrswitch

import (
	"fmt"
)

// Analyze evaluates AU-level HDR metadata and GOP safety. Malformed evidence
// returns an error; semantic stream/player violations are in Report.Findings.
func Analyze(evidence Evidence) (*Report, error) {
	if len(evidence.AccessUnits) == 0 {
		return nil, fmt.Errorf("access_units must contain at least one access unit")
	}

	switchAt, err := NewRational(evidence.SwitchAt.Num, evidence.SwitchAt.Den)
	if err != nil {
		return nil, fmt.Errorf("switch_at: %w", err)
	}
	if switchAt.Num < 0 {
		return nil, fmt.Errorf("switch_at must be non-negative")
	}

	defaultBitDepth := evidence.BitDepth
	if defaultBitDepth == 0 {
		defaultBitDepth = 10
	}
	defaultFullRange := false
	if evidence.FullRange != nil {
		defaultFullRange = *evidence.FullRange
	}

	var frameDuration *Rational
	if evidence.FrameRate != nil {
		fps, err := NewRational(evidence.FrameRate.Num, evidence.FrameRate.Den)
		if err != nil {
			return nil, fmt.Errorf("frame_rate: %w", err)
		}
		if fps.Num <= 0 || fps.Den <= 0 {
			return nil, fmt.Errorf("frame_rate must be positive")
		}
		duration := Rational{Num: fps.Den, Den: fps.Num}.normalize()
		frameDuration = &duration
	}

	positionCount := len(evidence.AccessUnits)
	ptsValues := make([]Rational, positionCount)
	resolvedMetadata := make([]ColorMetadata, positionCount)
	states := make([]AUState, positionCount)
	findings := make([]Finding, 0, 16)
	seenIndexes := make(map[int64]bool, positionCount)

	currentMetadata := ColorMetadata{
		BitDepth:  defaultBitDepth,
		FullRange: boolPtr(defaultFullRange),
	}

	for position, au := range evidence.AccessUnits {
		if seenIndexes[au.Index] {
			return nil, fmt.Errorf("duplicate access_unit index %d", au.Index)
		}
		seenIndexes[au.Index] = true

		pts, err := resolvePTS(au, evidence.PTSTimeBase)
		if err != nil {
			return nil, err
		}
		ptsValues[position] = pts

		var dts *Rational
		if au.DTS != nil {
			normalized, err := NewRational(au.DTS.Num, au.DTS.Den)
			if err != nil {
				return nil, fmt.Errorf("AU %d dts: %w", au.Index, err)
			}
			dts = &normalized
		}

		currentMetadata = mergeMetadata(currentMetadata, au.Metadata)
		if currentMetadata.BitDepth == 0 {
			currentMetadata.BitDepth = defaultBitDepth
		}
		if currentMetadata.FullRange == nil {
			currentMetadata.FullRange = boolPtr(defaultFullRange)
		}

		validationFindings := validateColorMetadata(au.Index, currentMetadata)
		for _, finding := range validationFindings {
			if finding.Code == "invalid_transfer_metadata" || finding.Code == "invalid_gamut_metadata" {
				return nil, fmt.Errorf("%s", finding.Message)
			}
		}
		findings = append(findings, validationFindings...)

		changedFields := []string{}
		metadataChanged := position == 0 && au.Metadata != nil
		if position > 0 {
			changedFields = diffMetadata(resolvedMetadata[position-1], currentMetadata)
			metadataChanged = len(changedFields) > 0
		}

		segment := SegmentBefore
		if pts.Cmp(switchAt) >= 0 {
			segment = SegmentAfter
		}

		resolvedMetadata[position] = currentMetadata
		states[position] = AUState{
			Index: au.Index, Position: position, PTS: pts, DTS: dts,
			Segment: segment, GOPID: au.GOPID, IDR: au.IDR,
			RandomAccess:    au.RandomAccess || au.IDR,
			MetadataChanged: metadataChanged, ChangedFields: changedFields,
			Metadata: currentMetadata,
		}
	}

	cutPosition := chooseCutPosition(ptsValues, switchAt)
	if cutPosition < 0 {
		return nil, fmt.Errorf("no access unit has PTS at or after switch_at=%s", switchAt.SecondsText(6))
	}
	beforePosition := chooseLastBeforePosition(ptsValues, switchAt)
	if beforePosition < 0 {
		return nil, fmt.Errorf("no access unit has PTS before switch_at=%s", switchAt.SecondsText(6))
	}

	cutAlignment := "exact_access_unit"
	if ptsValues[cutPosition].Cmp(switchAt) != 0 {
		cutAlignment = "between_access_units"
		cutIndex := evidence.AccessUnits[cutPosition].Index
		findings = append(findings, Finding{
			Code: "switch_not_au_aligned", Severity: SeverityError, AUIndex: &cutIndex,
			Message: fmt.Sprintf("switch_at=%s is not an AU/PTS boundary; next AU %d starts at %s. Transfer metadata cannot be frame-accurately applied between access units.",
				switchAt.SecondsText(6), cutIndex, ptsValues[cutPosition].SecondsText(6)),
		})
	}
	if frameDuration != nil {
		timelineStart := minimumRational(ptsValues)
		frameNumber := switchAt.Sub(timelineStart).Div(*frameDuration)
		if frameNumber.Den != 1 {
			findings = append(findings, Finding{
				Code: "switch_time_not_frame_aligned", Severity: SeverityError,
				Message: fmt.Sprintf("switch_at is %s frame durations from timeline start; use an integer AU boundary", frameNumber.String()),
			})
		}
	}

	report := &Report{
		OK: true, SwitchAt: switchAt, CutAlignment: cutAlignment,
		AccessUnits: states, Findings: findings,
	}
	cutIndex := states[cutPosition].Index
	report.CutAUIndex = &cutIndex

	gopRisk, gopFindings := analyzeGOP(evidence, states, cutPosition)
	report.GOPRisk = gopRisk
	report.Findings = append(gopFindings, report.Findings...)
	states[cutPosition].IsCutAU = true
	states[cutPosition].BoundarySafe = gopRisk != nil && gopRisk.Safe
	report.AccessUnits = states

	beforeSegment, afterSegment, regimes, segmentFindings, err := buildSegments(evidence, states, resolvedMetadata, ptsValues, switchAt, frameDuration)
	if err != nil {
		return nil, err
	}
	report.Before = beforeSegment
	report.After = afterSegment
	report.Regimes = regimes
	report.Findings = append(report.Findings, segmentFindings...)

	applyPlayerSticky(evidence, resolvedMetadata, states, report)
	validateExpectedTransfers(evidence, report)

	for i := range report.Findings {
		if report.Findings[i].Severity == SeverityError {
			report.OK = false
			break
		}
	}
	report.Classification, report.Summary = classify(report)
	return report, nil
}

func resolvePTS(au AccessUnit, defaultTimeBase *Rational) (Rational, error) {
	if au.PTSTicks != nil {
		timeBase := defaultTimeBase
		if au.TimeBase != nil {
			timeBase = au.TimeBase
		}
		if timeBase == nil {
			return Rational{}, fmt.Errorf("AU %d has pts_ticks but no time_base", au.Index)
		}
		tb, err := NewRational(timeBase.Num, timeBase.Den)
		if err != nil {
			return Rational{}, fmt.Errorf("AU %d time_base: %w", au.Index, err)
		}
		return MustRational(*au.PTSTicks, 1).Mul(tb), nil
	}
	if au.PTS == nil {
		return Rational{}, fmt.Errorf("AU %d requires pts or pts_ticks", au.Index)
	}
	return NewRational(au.PTS.Num, au.PTS.Den)
}

func chooseCutPosition(pts []Rational, at Rational) int {
	position := -1
	for i, ptsValue := range pts {
		if ptsValue.Cmp(at) < 0 {
			continue
		}
		if position == -1 || ptsValue.Cmp(pts[position]) < 0 || (ptsValue.Cmp(pts[position]) == 0 && i < position) {
			position = i
		}
	}
	return position
}

func chooseLastBeforePosition(pts []Rational, at Rational) int {
	position := -1
	for i, ptsValue := range pts {
		if ptsValue.Cmp(at) >= 0 {
			continue
		}
		if position == -1 || ptsValue.Cmp(pts[position]) > 0 || (ptsValue.Cmp(pts[position]) == 0 && i > position) {
			position = i
		}
	}
	return position
}

func minimumRational(values []Rational) Rational {
	minimum := values[0]
	for _, value := range values[1:] {
		if value.Cmp(minimum) < 0 {
			minimum = value
		}
	}
	return minimum
}

func validateExpectedTransfers(evidence Evidence, report *Report) {
	check := func(expected string, segment *SegmentReport) {
		if expected == "" || segment == nil {
			return
		}
		actual := segment.BoundaryMetadata.Transfer
		canonical, _, err := normalizeTransfer(&segment.BoundaryMetadata)
		if err == nil {
			actual = canonical
		}
		expectedCanonical := expected
		if normalized, _, err := normalizeTransfer(&ColorMetadata{Transfer: expected}); err == nil {
			expectedCanonical = normalized
		}
		if actual != expectedCanonical {
			severity := SeverityError
			report.Findings = append(report.Findings, Finding{
				Code: "expected_transfer_mismatch", Severity: severity,
				Message: fmt.Sprintf("%s boundary transfer is %s, expected %s", segment.ID, actual, expectedCanonical),
			})
			report.OK = false
		}
	}
	check(evidence.ExpectedBeforeTransfer, report.Before)
	check(evidence.ExpectedAfterTransfer, report.After)
}

func classify(report *Report) (string, string) {
	switch {
	case report.GOPRisk != nil && (report.GOPRisk.AtRisk || !report.GOPRisk.Safe):
		return "unsafe_gop_switch", "The ten-minute metadata change is not safely isolated at an IDR/random-access boundary; report and fix the GOP/splice risk before relabeling transfer metadata."
	case report.Player != nil && report.Player.MismatchAUCount > 0:
		return "player_stale_first_frame_metadata", "Access units carry changed HDR metadata, but the player/global label keeps using the first-frame value."
	case report.Before != nil && !report.Before.Stable || report.After != nil && !report.After.Stable:
		return "multiple_color_regimes", "Each cut side contains more than one color metadata regime; use Regimes for per-AU legal ranges."
	case hasErrorFinding(report.Findings):
		return "invalid_hdr_metadata", "The AU timeline contains invalid or inconsistent HDR metadata or switch alignment."
	default:
		return "valid_au_aligned_switch", "The switch is AU/GOP-isolated and AU metadata supplies the correct transfer, peak, and gamut range."
	}
}

func hasErrorFinding(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Severity == SeverityError {
			return true
		}
	}
	return false
}
