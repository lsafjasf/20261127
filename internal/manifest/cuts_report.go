package manifest

import "fmt"

func (a *assessor) buildCuts(hdrProbe, sdrProbe ProbeData) (CutsReport, CutsReport) {
	hdrMarkers, hdrMalformed := CutMarkersFromProbe(hdrProbe)
	sdrMarkers, sdrMalformed := CutMarkersFromProbe(sdrProbe)
	tolerance := CutToleranceMS(a.cfg.Cuts)

	if a.cutMode == "paired" {
		hdrExpected := markersToPoints(hdrMarkers)
		hdrReport := a.makeCutsReport(
			"paired",
			hdrExpected,
			hdrMarkers,
			hdrMalformed,
			true,
			a.cfg.Cuts.AllowExtra,
			tolerance,
		)
		sdrReport := a.makeCutsReport(
			"paired",
			hdrExpected,
			sdrMarkers,
			sdrMalformed,
			false,
			a.cfg.Cuts.AllowExtra,
			tolerance,
		)
		return hdrReport, sdrReport
	}

	hdrReport := a.makeCutsReport(
		"list",
		a.cutPoints,
		hdrMarkers,
		hdrMalformed,
		false,
		a.cfg.Cuts.AllowExtra,
		tolerance,
	)
	sdrReport := a.makeCutsReport(
		"list",
		a.cutPoints,
		sdrMarkers,
		sdrMalformed,
		false,
		a.cfg.Cuts.AllowExtra,
		tolerance,
	)
	return hdrReport, sdrReport
}

func (a *assessor) makeCutsReport(mode string, expected []CutPoint, actual []CutMarker, malformed int, baseline bool, allowExtra bool, toleranceMS int64) CutsReport {
	missing, unexpected := MatchCuts(expected, actual, toleranceMS)
	report := CutsReport{
		Mode:         mode,
		Status:       StatusIncomplete,
		ToleranceMS:  toleranceMS,
		ChapterCount: len(actual),
		Malformed:    malformed,
		Expected:     append([]CutPoint{}, expected...),
		Actual:       append([]CutMarker{}, actual...),
		Missing:      missing,
		Unexpected:   unexpected,
		Checks:       []Check{},
	}

	if baseline {
		report.Checks = append(report.Checks, Check{
			ID:       "cuts.baseline_present",
			Category: "cuts",
			Required: true,
			Expected: "at least one HDR cut marker",
			Actual:   len(actual),
			Pass:     len(actual) > 0,
			Detail:   emptyCutsDetail(len(actual)),
		})
	} else if mode == "paired" {
		report.Checks = append(report.Checks, Check{
			ID:       "cuts.paired_baseline",
			Category: "cuts",
			Required: true,
			Expected: "non-empty HDR cut baseline",
			Actual:   len(expected),
			Pass:     len(expected) > 0,
			Detail:   pairedBaselineDetail(len(expected)),
		})
	}
	report.Checks = append(report.Checks,
		Check{
			ID:       "cuts.chapter_times",
			Category: "cuts",
			Required: true,
			Expected: "all chapter start times parse",
			Actual:   malformed,
			Pass:     malformed == 0,
			Detail:   malformedCutDetail(malformed),
		},
		Check{
			ID:       "cuts.missing",
			Category: "cuts",
			Required: true,
			Expected: 0,
			Actual:   len(missing),
			Pass:     len(missing) == 0,
			Detail:   missingCutsDetail(missing),
		},
	)
	if !allowExtra {
		report.Checks = append(report.Checks, Check{
			ID:       "cuts.unexpected",
			Category: "cuts",
			Required: true,
			Expected: 0,
			Actual:   len(unexpected),
			Pass:     len(unexpected) == 0,
			Detail:   unexpectedCutsDetail(unexpected),
		})
	}
	report.Status = statusFromChecks(report.Checks)
	return report
}

func markersToPoints(markers []CutMarker) []CutPoint {
	points := make([]CutPoint, 0, len(markers))
	for i, marker := range markers {
		id := marker.ID
		if id == "" {
			id = fmt.Sprintf("chapter-%d", i+1)
		}
		points = append(points, CutPoint{
			ID:     id,
			TimeMS: marker.TimeMS,
			Label:  marker.Label,
		})
	}
	return points
}

func emptyCutsDetail(count int) string {
	if count > 0 {
		return ""
	}
	return "no cut markers are present in the paired baseline"
}

func pairedBaselineDetail(count int) string {
	if count > 0 {
		return ""
	}
	return "cannot verify SDR cut points because the HDR baseline has no cut markers"
}

func malformedCutDetail(count int) string {
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("%d chapter marker(s) have no parseable start time", count)
}

func missingCutsDetail(missing []CutPoint) string {
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("%d expected cut point(s) are missing", len(missing))
}

func unexpectedCutsDetail(unexpected []CutMarker) string {
	if len(unexpected) == 0 {
		return ""
	}
	return fmt.Sprintf("%d cut point(s) are not in the expected list", len(unexpected))
}
