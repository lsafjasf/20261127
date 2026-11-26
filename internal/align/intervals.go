package align

import (
	"fmt"
	"math"
	"sort"
)

type span struct {
	start float64
	end   float64
}

func buildSegmentReport(state *segmentState, idx *indexedSession) SegmentReport {
	segment := state.segment
	report := SegmentReport{
		Device: state.device.ID, ID: segment.ID,
		LocalStart: segment.Start, LocalEnd: segment.End,
		Status: state.status, Map: []MapPiece{}, Indeterminate: []Interval{},
	}
	if state.status == "master" {
		return masterReport(report)
	}
	if state.fit == nil {
		reason := "no_usable_anchors"
		detail := fmt.Sprintf("%d raw anchor(s) available", len(state.anchors))
		if len(state.anchors) > 0 {
			reason = "inconsistent_anchors"
			detail = "anchors exist but no drift-plausible consensus was found"
		}
		report.Indeterminate = []Interval{{
			LocalStart: segment.Start, LocalEnd: segment.End,
			Reason: reason, Detail: detail,
		}}
		return report
	}

	report.Fit = fitInfo(state.fit)
	offset := rounded(state.fit.mapTime(segment.Start)-segment.Start, 6)
	report.OffsetAtStart = &offset
	if len(state.fit.inliers) >= 2 {
		drift := rounded((state.fit.slope-1)*1e6, 3)
		report.DriftPPM = &drift
	}
	report.DriftConfirmed = state.fit.driftConfirmed
	if state.fit.status == "locked" {
		first := state.fit.inliers[0]
		report.FirstTrusted = &AnchorInfo{
			Event: first.event, Kind: first.kind, Local: first.local,
			Master: first.master, OffsetSec: rounded(first.master-first.local, 6),
			Confidence: rounded(first.confidence, 6), ResidualMS: rounded(first.residualMS, 3),
		}
	}

	coverage, forced := trustedCoverage(state.fit, segment, idx.config)
	unknown := complement(segment.Start, segment.End, coverage)
	for _, gap := range forced {
		unknown = append(unknown, Interval{
			LocalStart: gap.start, LocalEnd: gap.end,
			Reason: "anchor_gap_too_large",
			Detail: fmt.Sprintf("gap exceeds %.1f s", idx.config.MaxAnchorGapSec),
		})
	}
	unknown = clampMerge(unknown, segment.Start, segment.End)
	unknown = annotateUnknown(unknown, state, idx.config)
	report.Indeterminate = unknown
	report.Map = mapPieces(segment, state.fit, unknown, idx.config)
	return report
}

func masterReport(report SegmentReport) SegmentReport {
	zero := 0.0
	report.OffsetAtStart = &zero
	report.Map = []MapPiece{{
		LocalStart: report.LocalStart, LocalEnd: report.LocalEnd,
		MasterStart: report.LocalStart, MasterEnd: report.LocalEnd,
		Slope: 1, Intercept: 0, MaxErrorMS: 0,
	}}
	return report
}

func fitInfo(fit *fitResult) *FitInfo {
	return &FitInfo{
		Slope: rounded(fit.slope, 9), Intercept: rounded(fit.intercept, 6),
		AnchorCount: len(fit.inliers), RejectedAnchors: fit.rejected,
		BaselineSec:   rounded(fit.baseline, 6),
		RMSResidualMS: rounded(fit.rmsMS, 3), MaxResidualMS: rounded(fit.maxMS, 3),
	}
}

func trustedCoverage(fit *fitResult, segment *Segment, cfg Config) ([]span, []span) {
	radius := coverageRadius(fit, cfg)
	var spans []span
	for _, point := range fit.inliers {
		start := math.Max(segment.Start, point.local-radius)
		end := math.Min(segment.End, point.local+radius)
		if end > start {
			spans = append(spans, span{start: start, end: end})
		}
	}
	spans = mergeSpans(spans)

	var forced []span
	for i := 1; i < len(fit.inliers); i++ {
		a, b := fit.inliers[i-1].local, fit.inliers[i].local
		if b-a > cfg.MaxAnchorGapSec {
			// A gap wider than the configured limit is unknown as a whole,
			// even if an optimistic radius would bridge part of it.
			forced = append(forced, span{start: a, end: b})
		}
	}
	return spans, forced
}

func coverageRadius(fit *fitResult, cfg Config) float64 {
	budget := cfg.MaxMappingErrorMS - cfg.BaseUncertaintyMS - fit.maxMS
	if budget <= 0 {
		return 0
	}
	driftPPM := cfg.MaxUnverifiedDriftPPM
	if fit.driftConfirmed {
		driftPPM = cfg.ConfirmedDriftPPM
	}
	if driftPPM <= 0 {
		return cfg.MaxExtrapolationSec
	}
	radius := budget * 1000 / driftPPM
	return math.Min(radius, cfg.MaxExtrapolationSec)
}

func mergeSpans(input []span) []span {
	if len(input) == 0 {
		return nil
	}
	sort.Slice(input, func(i, j int) bool { return input[i].start < input[j].start })
	merged := []span{input[0]}
	const epsilon = 1e-9
	for _, item := range input[1:] {
		last := &merged[len(merged)-1]
		if item.start <= last.end+epsilon {
			last.end = math.Max(last.end, item.end)
		} else {
			merged = append(merged, item)
		}
	}
	return merged
}

func complement(start, end float64, coverage []span) []Interval {
	var result []Interval
	cursor := start
	for _, item := range coverage {
		if item.start > cursor+1e-9 {
			result = append(result, Interval{LocalStart: cursor, LocalEnd: item.start})
		}
		cursor = math.Max(cursor, item.end)
	}
	if cursor < end-1e-9 {
		result = append(result, Interval{LocalStart: cursor, LocalEnd: end})
	}
	return result
}

func clampMerge(input []Interval, start, end float64) []Interval {
	sort.Slice(input, func(i, j int) bool { return input[i].LocalStart < input[j].LocalStart })
	var result []Interval
	for _, item := range input {
		item.LocalStart = math.Max(start, item.LocalStart)
		item.LocalEnd = math.Min(end, item.LocalEnd)
		if item.LocalEnd <= item.LocalStart {
			continue
		}
		if len(result) > 0 && item.LocalStart <= result[len(result)-1].LocalEnd+1e-9 {
			last := &result[len(result)-1]
			last.LocalEnd = math.Max(last.LocalEnd, item.LocalEnd)
			if item.Reason != "" && last.Reason != item.Reason {
				last.Reason = "multiple_causes"
			}
			continue
		}
		result = append(result, item)
	}
	return result
}

func annotateUnknown(intervals []Interval, state *segmentState, cfg Config) []Interval {
	for i := range intervals {
		if intervals[i].Reason != "" {
			continue
		}
		first := state.fit.inliers[0].local
		last := state.fit.inliers[len(state.fit.inliers)-1].local
		switch {
		case intervals[i].LocalEnd <= first+1e-9:
			intervals[i].Reason = "before_first_anchor_coverage"
			intervals[i].Detail = "insufficient evidence before the earliest usable anchor"
		case intervals[i].LocalStart >= last-1e-9:
			intervals[i].Reason = "after_last_anchor_coverage"
			intervals[i].Detail = "extrapolation exceeds the error budget"
		default:
			intervals[i].Reason = "anchor_coverage_gap"
			intervals[i].Detail = fmt.Sprintf("no anchor within %.1f s coverage radius", coverageRadius(state.fit, cfg))
		}
	}
	return intervals
}

func mapPieces(segment *Segment, fit *fitResult, unknown []Interval, cfg Config) []MapPiece {
	var coverage []span
	cursor := segment.Start
	for _, item := range unknown {
		if item.LocalStart > cursor+1e-9 {
			coverage = append(coverage, span{start: cursor, end: item.LocalStart})
		}
		cursor = math.Max(cursor, item.LocalEnd)
	}
	if cursor < segment.End-1e-9 {
		coverage = append(coverage, span{start: cursor, end: segment.End})
	}
	var result []MapPiece
	for _, item := range coverage {
		errorMS := maxError(item.start, item.end, fit, cfg)
		result = append(result, MapPiece{
			LocalStart: item.start, LocalEnd: item.end,
			MasterStart: fit.mapTime(item.start), MasterEnd: fit.mapTime(item.end),
			Slope: fit.slope, Intercept: fit.intercept, MaxErrorMS: rounded(errorMS, 3),
		})
	}
	return result
}

func maxError(start, end float64, fit *fitResult, cfg Config) float64 {
	distance := maxNearestAnchorDistance(start, end, fit.inliers)
	driftPPM := cfg.MaxUnverifiedDriftPPM
	if fit.driftConfirmed {
		driftPPM = cfg.ConfirmedDriftPPM
	}
	return cfg.BaseUncertaintyMS + fit.maxMS + distance*driftPPM/1000
}

func maxNearestAnchorDistance(start, end float64, anchors []anchor) float64 {
	points := []float64{start, end}
	for i := 1; i < len(anchors); i++ {
		mid := (anchors[i-1].local + anchors[i].local) / 2
		if mid > start && mid < end {
			points = append(points, mid)
		}
	}
	result := 0.0
	for _, point := range points {
		nearest := math.Inf(1)
		for _, evidence := range anchors {
			nearest = math.Min(nearest, math.Abs(point-evidence.local))
		}
		if !math.IsInf(nearest, 1) {
			result = math.Max(result, nearest)
		}
	}
	return result
}
