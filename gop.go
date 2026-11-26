package hdrswitch

import "fmt"

func policyTrue(value *bool) bool {
	return value == nil || *value
}

func analyzeGOP(evidence Evidence, states []AUState, cutPosition int) (*GOPRisk, []Finding) {
	cutState := states[cutPosition]
	cutAU := evidence.AccessUnits[cutPosition]
	requireIDR := policyTrue(evidence.Policy.RequireIDR)
	requireParameterSets := policyTrue(evidence.Policy.RequireParameterSetUpdate)

	previousPosition := cutPosition
	for previousPosition >= 0 && !states[previousPosition].RandomAccess {
		previousPosition--
	}
	nextPosition := cutPosition + 1
	for nextPosition < len(states) && !states[nextPosition].RandomAccess {
		nextPosition++
	}

	risk := &GOPRisk{
		CutAUIndex: cutState.Index,
		CutPTS:     cutState.PTS,
		GOPID:      cutState.GOPID,
	}

	if previousPosition >= 0 {
		previousAU := evidence.AccessUnits[previousPosition]
		previousState := states[previousPosition]
		risk.GOPStartAUIndex = &previousState.Index
		startPTS := previousState.PTS
		risk.GOPStartPTS = &startPTS
		risk.PositionFromGOPStart = cutState.Index - previousState.Index
		if risk.GOPID == "" {
			risk.GOPID = previousState.GOPID
		}
		if previousAU.ClosedGOP != nil && !*previousAU.ClosedGOP {
			risk.OpenGOP = true
		}
		if previousAU.CrossReference {
			risk.CrossBoundaryReference = true
		}
		risk.PreviousRandomAccess = boundaryFrom(previousAU, previousState)
	}
	if cutAU.ClosedGOP != nil && !*cutAU.ClosedGOP {
		risk.OpenGOP = true
	}
	if cutAU.CrossReference {
		risk.CrossBoundaryReference = true
	}
	if nextPosition < len(states) {
		nextAU := evidence.AccessUnits[nextPosition]
		risk.NextRandomAccess = boundaryFrom(nextAU, states[nextPosition])
	}
	if risk.GOPID == "" && risk.GOPStartAUIndex != nil {
		risk.GOPID = fmt.Sprintf("from-au-%d", *risk.GOPStartAUIndex)
	}

	findings := []Finding{}
	cutIndex := cutState.Index
	add := func(code, severity, message string) {
		findings = append(findings, Finding{Code: code, Severity: severity, AUIndex: &cutIndex, Message: message})
	}

	structuralBoundary := cutState.IDR || cutState.RandomAccess
	closedRAP := !risk.OpenGOP && !risk.CrossBoundaryReference

	switch {
	case !structuralBoundary:
		risk.AtRisk = true
		risk.Safe = false
		risk.Reason = "cut AU is not an IDR/RAP; P/B slices in the same GOP can reference pre-switch pictures"
		risk.RequiredAction = "Re-encode so an IDR or explicitly splice-safe closed-GOP random-access CVS starts at the ten-minute point, with new SPS/VUI/PPS and HDR SEI; alternatively move the switch to PreviousRandomAccess or NextRandomAccess. Changing only a global/container label leaves decode references and player metadata stale."
		add("switch_inside_gop", SeverityError, fmt.Sprintf(
			"switch AU %d at %s lies inside GOP %q (GOP start AU %s, offset %d AU); do not relabel globally before fixing the splice boundary",
			cutIndex, cutState.PTS.SecondsText(6), risk.GOPID,
			indexText(risk.GOPStartAUIndex), risk.PositionFromGOPStart,
		))

	case risk.OpenGOP || risk.CrossBoundaryReference:
		risk.AtRisk = true
		risk.Safe = false
		risk.Reason = "random-access candidate is an open GOP or has cross-boundary references/RASL dependencies"
		risk.RequiredAction = "Use an IDR or a verified closed-GOP boundary with no cross-boundary references, then repeat parameter sets and HDR metadata."
		add("open_gop_cross_reference", SeverityError, fmt.Sprintf(
			"switch AU %d is a RAP but open_gop=%v cross_boundary_reference=%v; bitstream splicing may decode using pre-switch references",
			cutIndex, risk.OpenGOP, risk.CrossBoundaryReference,
		))

	case !cutState.IDR && requireIDR:
		risk.AtRisk = false
		risk.Safe = false
		risk.Reason = "cut AU is a non-IDR RAP; default delivery policy requires IDR for frame-accurate metadata switching"
		risk.RequiredAction = "Insert an IDR at the switch or set policy.require_idr=false only after verifying closed-GOP/RASL behavior and player metadata reset."
		add("idr_required_for_metadata_switch", SeverityError, fmt.Sprintf(
			"switch AU %d is %s rather than IDR; it may be a valid RAP but does not satisfy the safe IDR/CVS switch policy",
			cutIndex, rapLabel(cutAU),
		))

	default:
		risk.Reason = "cut AU is a closed random-access boundary"
		risk.Safe = closedRAP
	}

	if structuralBoundary && risk.Safe && requireParameterSets && !cutAU.ParameterSets {
		risk.Safe = false
		risk.RequiredAction = "Write/activate new SPS/VUI with colour_primaries/transfer characteristics, PPS, and repeating mastering/CLL SEI at the switch IDR/CVS."
		add("parameter_set_update_missing", SeverityError, fmt.Sprintf(
			"switch AU %d is structurally random-access but parameter_sets_updated=false; H.273 transfer/primaries live in VUI/SPS and HDR peaks in SEI, so a global label alone is insufficient",
			cutIndex,
		))
	}

	if risk.Safe {
		add("safe_au_boundary", SeverityInfo, fmt.Sprintf(
			"switch AU %d is %s and marked with updated parameter sets; AU-local metadata can take effect at this boundary",
			cutIndex, rapLabel(cutAU),
		))
	}

	return risk, findings
}

func boundaryFrom(au AccessUnit, state AUState) *Boundary {
	openGOP := au.ClosedGOP != nil && !*au.ClosedGOP
	safe := state.IDR || (state.RandomAccess && !openGOP && !au.CrossReference)
	boundary := &Boundary{
		AUIndex: state.Index, PTS: state.PTS, IDR: state.IDR,
		Type: rapLabel(au), Safe: safe,
	}
	return boundary
}

func rapLabel(au AccessUnit) string {
	if au.RAPType != "" {
		return au.RAPType
	}
	if au.IDR {
		return "IDR"
	}
	if au.RandomAccess {
		return "RAP"
	}
	return "non-RAP"
}

func indexText(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *value)
}
