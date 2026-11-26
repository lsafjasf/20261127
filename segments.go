package hdrswitch

import (
	"fmt"
	"sort"
)

func buildSegments(
	evidence Evidence,
	states []AUState,
	metas []ColorMetadata,
	pts []Rational,
	switchAt Rational,
	frameDuration *Rational,
) (*SegmentReport, *SegmentReport, []Regime, []Finding, error) {
	ordered := make([]int, len(pts))
	for i := range ordered {
		ordered[i] = i
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return pts[ordered[i]].Cmp(pts[ordered[j]]) < 0
	})

	beforePositions := []int{}
	afterPositions := []int{}
	for _, position := range ordered {
		if pts[position].Cmp(switchAt) < 0 {
			beforePositions = append(beforePositions, position)
		} else {
			afterPositions = append(afterPositions, position)
		}
	}
	if len(beforePositions) == 0 || len(afterPositions) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("switch_at must split access units into non-empty before/after sets")
	}

	findings := []Finding{}
	regimes := []Regime{}
	beforeSegment, beforeRegimes, beforeFindings, err := buildSegment(
		SegmentBefore, beforePositions, afterPositions, states, metas, pts, frameDuration, evidence.TimelineComplete, &regimes,
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	findings = append(findings, beforeFindings...)

	afterSegment, afterRegimes, afterFindings, err := buildSegment(
		SegmentAfter, afterPositions, nil, states, metas, pts, frameDuration, evidence.TimelineComplete, &regimes,
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	findings = append(findings, afterFindings...)
	_ = beforeRegimes
	_ = afterRegimes

	return beforeSegment, afterSegment, regimes, findings, nil
}

func buildSegment(
	segmentID string,
	positions []int,
	followingPositions []int,
	states []AUState,
	metas []ColorMetadata,
	pts []Rational,
	frameDuration *Rational,
	timelineComplete bool,
	allRegimes *[]Regime,
) (*SegmentReport, []int, []Finding, error) {
	firstPosition := positions[0]
	lastPosition := positions[len(positions)-1]
	boundaryPosition := lastPosition
	if segmentID == SegmentAfter {
		boundaryPosition = firstPosition
	}

	envelope, err := buildEnvelope(metas[boundaryPosition], metas[boundaryPosition].BitDepth, boolValue(metas[boundaryPosition].FullRange, false))
	if err != nil {
		return nil, nil, nil, err
	}

	segment := &SegmentReport{
		ID: segmentID, StartPTS: pts[firstPosition],
		AUCount: len(positions), FirstAUIndex: states[firstPosition].Index,
		LastAUIndex:      states[lastPosition].Index,
		BoundaryMetadata: metas[boundaryPosition], LegalRange: envelope,
		RegimeIDs: []int{},
	}

	if segmentID == SegmentBefore && len(followingPositions) > 0 {
		end := pts[followingPositions[0]]
		segment.EndPTS = &end
	} else if timelineComplete && frameDuration != nil {
		end := pts[lastPosition].Add(*frameDuration)
		segment.EndPTS = &end
	}

	segment.Transfers = canonicalTransfers(positions, metas)
	segment.Primaries = canonicalPrimaries(positions, metas)
	segment.PeaksNits = peakLabels(positions, metas)

	regimePositions := []int{}
	findings := []Finding{}
	regimeStart := 0
	regimeIndex := len(*allRegimes)

	flush := func(endExclusive int) error {
		if regimeStart >= endExclusive {
			return nil
		}
		group := positions[regimeStart:endExclusive]
		groupFirst := group[0]
		groupLast := group[len(group)-1]
		groupEnvelope, err := buildEnvelope(metas[groupFirst], metas[groupFirst].BitDepth, boolValue(metas[groupFirst].FullRange, false))
		if err != nil {
			return err
		}

		var endPTS *Rational
		if endExclusive < len(positions) {
			end := pts[positions[endExclusive]]
			endPTS = &end
		} else if segmentID == SegmentBefore && len(followingPositions) > 0 {
			end := pts[followingPositions[0]]
			endPTS = &end
		} else if timelineComplete && frameDuration != nil {
			end := pts[groupLast].Add(*frameDuration)
			endPTS = &end
		}

		regimeIndex++
		regime := Regime{
			ID: regimeIndex, Segment: segmentID,
			FirstAUIndex: states[groupFirst].Index, LastAUIndex: states[groupLast].Index,
			StartPTS: pts[groupFirst], EndPTS: endPTS, AUCount: len(group),
			Metadata: metas[groupFirst], Envelope: groupEnvelope,
		}
		*allRegimes = append(*allRegimes, regime)
		segment.RegimeIDs = append(segment.RegimeIDs, regime.ID)
		regimePositions = append(regimePositions, groupFirst)
		regimeStart = endExclusive
		return nil
	}

	for i := 1; i <= len(positions); i++ {
		if i == len(positions) {
			if err := flush(i); err != nil {
				return nil, nil, nil, err
			}
			break
		}
		reference := metas[positions[regimeStart]]
		current := metas[positions[i]]
		if len(diffMetadata(reference, current)) > 0 {
			if err := flush(i); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	segment.Stable = len(segment.RegimeIDs) == 1
	if !segment.Stable {
		severity := SeverityInfo
		code := "multiple_peak_or_metadata_regimes"
		message := fmt.Sprintf("%s side has %d AU metadata regimes; top-level legal_range is cut-adjacent only, use regimes[] for each interval", segmentID, len(segment.RegimeIDs))
		if len(segment.Transfers) > 1 || len(segment.Primaries) > 1 {
			severity = SeverityWarning
			code = "transfer_or_gamut_changed_within_segment"
			message = fmt.Sprintf("%s side changes transfer=%v or gamut=%v; it does not have one single legal range", segmentID, segment.Transfers, segment.Primaries)
		}
		findings = append(findings, Finding{Code: code, Severity: severity, Message: message})
	}

	return segment, regimePositions, findings, nil
}

func canonicalTransfers(positions []int, metas []ColorMetadata) []string {
	values := []string{}
	for _, position := range positions {
		if canonical, _, err := normalizeTransfer(&metas[position]); err == nil {
			values = append(values, canonical)
		} else {
			values = append(values, metas[position].Transfer)
		}
	}
	return uniqueStrings(values)
}

func canonicalPrimaries(positions []int, metas []ColorMetadata) []string {
	values := []string{}
	for _, position := range positions {
		if profile, err := normalizePrimaries(&metas[position]); err == nil {
			values = append(values, profile.Name)
		} else {
			values = append(values, metas[position].Primaries)
		}
	}
	return uniqueStrings(values)
}

func peakLabels(positions []int, metas []ColorMetadata) []string {
	values := []*Rational{}
	for _, position := range positions {
		meta := metas[position]
		switch {
		case meta.NominalPeakNits != nil:
			values = append(values, meta.NominalPeakNits)
		case meta.MaxCLL != nil:
			values = append(values, meta.MaxCLL)
		case meta.Mastering != nil && meta.Mastering.MaxLuminanceNits != nil:
			values = append(values, meta.Mastering.MaxLuminanceNits)
		}
	}
	return rationalStrings(values)
}
