package manifest

type scopedCheck struct {
	Scope   string
	Section string
	Check   Check
}

func statusFromChecks(checks []Check) string {
	for _, check := range checks {
		if check.Required && !check.Pass {
			return StatusIncomplete
		}
	}
	return StatusComplete
}

func (a *assessor) finalize(report *Report) {
	report.Versions.HDR.Status = statusFromChecks(versionChecks(report.Versions.HDR))
	report.Versions.SDR.Status = statusFromChecks(versionChecks(report.Versions.SDR))

	sharedChecks := collectSharedChecks(report)
	report.Shared.Status = statusFromChecks(sharedChecks)

	all := collectAllChecks(report)
	report.Summary = Summary{
		TotalChecks:  len(all),
		HDRStatus:    report.Versions.HDR.Status,
		SDRStatus:    report.Versions.SDR.Status,
		SharedStatus: report.Shared.Status,
	}
	for _, item := range all {
		if item.Check.Pass {
			report.Summary.PassedChecks++
			continue
		}
		report.Summary.FailedChecks++
		report.Failures = append(report.Failures, Failure{
			Scope:    item.Scope,
			CheckID:  item.Check.ID,
			Category: item.Check.Category,
			Detail:   item.Check.Detail,
			Expected: item.Check.Expected,
			Actual:   item.Check.Actual,
		})
	}
	report.Complete = report.Summary.FailedChecks == 0 &&
		report.Versions.HDR.Status == StatusComplete &&
		report.Versions.SDR.Status == StatusComplete &&
		report.Shared.Status == StatusComplete
	if report.Complete {
		report.Status = StatusComplete
	} else {
		report.Status = StatusIncomplete
	}
}

func versionChecks(version VersionReport) []Check {
	var checks []Check
	checks = append(checks, version.Checks...)
	checks = append(checks, version.Color.Checks...)
	checks = append(checks, version.Cuts.Checks...)
	checks = append(checks, version.Defaults.Checks...)
	return checks
}

func collectSharedChecks(report *Report) []Check {
	var checks []Check
	checks = append(checks, report.Shared.Checks...)
	checks = append(checks, report.Shared.Timeline.Checks...)
	checks = append(checks, report.Shared.Audio.Checks...)
	checks = append(checks, report.Shared.Subtitles.Checks...)
	for _, asset := range report.Shared.Assets {
		checks = append(checks, asset.Checks...)
	}
	return checks
}

func collectAllChecks(report *Report) []scopedCheck {
	var all []scopedCheck
	for _, version := range []struct {
		scope string
		value VersionReport
	}{
		{scope: "hdr", value: report.Versions.HDR},
		{scope: "sdr", value: report.Versions.SDR},
	} {
		for _, check := range version.value.Checks {
			all = append(all, scopedCheck{Scope: version.scope, Section: check.Category, Check: check})
		}
		for _, check := range version.value.Color.Checks {
			all = append(all, scopedCheck{Scope: version.scope, Section: "color", Check: check})
		}
		for _, check := range version.value.Cuts.Checks {
			all = append(all, scopedCheck{Scope: version.scope, Section: "cuts", Check: check})
		}
		for _, check := range version.value.Defaults.Checks {
			all = append(all, scopedCheck{Scope: version.scope, Section: "default_tracks", Check: check})
		}
	}
	for _, check := range report.Shared.Checks {
		all = append(all, scopedCheck{Scope: "shared", Section: check.Category, Check: check})
	}
	for _, check := range report.Shared.Timeline.Checks {
		all = append(all, scopedCheck{Scope: "shared", Section: "timeline", Check: check})
	}
	for _, check := range report.Shared.Audio.Checks {
		all = append(all, scopedCheck{Scope: "shared", Section: "shared_tracks", Check: check})
	}
	for _, check := range report.Shared.Subtitles.Checks {
		all = append(all, scopedCheck{Scope: "shared", Section: "shared_tracks", Check: check})
	}
	for _, asset := range report.Shared.Assets {
		for _, check := range asset.Checks {
			all = append(all, scopedCheck{
				Scope:   "asset:" + asset.ID,
				Section: "shared_asset",
				Check:   check,
			})
		}
	}
	return all
}
