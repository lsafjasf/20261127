package manifest

import (
	"fmt"
	"strings"
)

func buildDefaultSummary(tracks []TrackSummary) DefaultsReport {
	report := DefaultsReport{Status: StatusIncomplete, Checks: []Check{}}
	for _, kind := range []string{"audio", "subtitle"} {
		defaults := tracksWithDefault(tracks, kind)
		check := Check{
			ID:       "defaults." + kind + "_count",
			Category: "default_tracks",
			Required: true,
			Expected: 1,
			Actual:   len(defaults),
		}
		check.Pass = len(defaults) == 1
		if !check.Pass {
			switch len(defaults) {
			case 0:
				check.Detail = "no default " + kind + " track"
			default:
				check.Detail = fmt.Sprintf("%d default %s tracks; exactly one is required", len(defaults), kind)
			}
		}
		report.Checks = append(report.Checks, check)
		if len(defaults) > 0 {
			track := defaults[0]
			if kind == "audio" {
				report.Audio = &track
			} else {
				report.Subtitle = &track
			}
		}
	}
	report.Status = statusFromChecks(report.Checks)
	return report
}

func tracksWithDefault(tracks []TrackSummary, kind string) []TrackSummary {
	var defaults []TrackSummary
	for _, track := range tracks {
		if strings.EqualFold(track.Type, kind) && track.Default {
			defaults = append(defaults, track)
		}
	}
	return defaults
}

func (a *assessor) buildDefaults(hdr, sdr VersionReport) (DefaultsReport, DefaultsReport) {
	hdrReport := buildDefaultSummary(hdr.EmbeddedTracks)
	sdrReport := buildDefaultSummary(sdr.EmbeddedTracks)
	startTolerance := a.startToleranceMS()
	durationTolerance := a.durationToleranceMS()

	hdrReport.Checks = append(hdrReport.Checks,
		sharedDefaultCheck("defaults.audio_shared", "audio", hdrReport.Audio, sdrReport.Audio),
		sharedDefaultCheck("defaults.subtitle_shared", "subtitle", hdrReport.Subtitle, sdrReport.Subtitle),
		sharedDefaultTimingCheck("defaults.audio_timing", "audio", hdrReport.Audio, sdrReport.Audio, startTolerance, durationTolerance),
		sharedDefaultTimingCheck("defaults.subtitle_timing", "subtitle", hdrReport.Subtitle, sdrReport.Subtitle, startTolerance, durationTolerance),
	)
	sdrReport.Checks = append(sdrReport.Checks,
		sharedDefaultCheck("defaults.audio_shared", "audio", sdrReport.Audio, hdrReport.Audio),
		sharedDefaultCheck("defaults.subtitle_shared", "subtitle", sdrReport.Subtitle, hdrReport.Subtitle),
		sharedDefaultTimingCheck("defaults.audio_timing", "audio", sdrReport.Audio, hdrReport.Audio, startTolerance, durationTolerance),
		sharedDefaultTimingCheck("defaults.subtitle_timing", "subtitle", sdrReport.Subtitle, hdrReport.Subtitle, startTolerance, durationTolerance),
	)
	hdrReport.Status = statusFromChecks(hdrReport.Checks)
	sdrReport.Status = statusFromChecks(sdrReport.Checks)
	return hdrReport, sdrReport
}

func sharedDefaultCheck(id, kind string, own, other *TrackSummary) Check {
	check := Check{
		ID:       id,
		Category: "default_tracks",
		Required: true,
		Expected: "same default " + kind + " track in HDR and SDR",
	}
	if own == nil || other == nil {
		check.Actual = "default track unavailable"
		check.Detail = "both versions must expose one default " + kind + " track"
		return check
	}
	ownSignature := trackShareSignature(*own, kind)
	otherSignature := trackShareSignature(*other, kind)
	check.Actual = map[string]string{
		"this_version":  ownSignature,
		"other_version": otherSignature,
	}
	check.Pass = own.StreamSHA256 != "" && other.StreamSHA256 != "" && ownSignature == otherSignature
	if !check.Pass {
		check.Detail = "default " + kind + " track content or metadata differs between versions"
	}
	return check
}

func trackShareSignature(track TrackSummary, kind string) string {
	parts := []string{
		track.StreamSHA256,
		strings.ToLower(track.Codec),
		strings.ToLower(track.Language),
		track.Title,
	}
	if kind == "audio" {
		parts = append(parts,
			fmt.Sprintf("%d", track.Channels),
			strings.ToLower(track.ChannelLayout),
			fmt.Sprintf("%d", track.SampleRate),
		)
	}
	return strings.Join(parts, "|")
}

func sharedDefaultTimingCheck(id, kind string, own, other *TrackSummary, startTolerance, durationTolerance int64) Check {
	check := Check{
		ID:       id,
		Category: "default_tracks",
		Required: true,
		Expected: fmt.Sprintf("start delta <= %d ms and duration delta <= %d ms when exposed", startTolerance, durationTolerance),
	}
	if own == nil || other == nil {
		check.Actual = "default track unavailable"
		check.Detail = "both versions must expose one default " + kind + " track"
		return check
	}
	check.Pass = true
	actual := map[string]any{}
	if own.StartAvailable && other.StartAvailable {
		delta := absInt64(own.StartMS - other.StartMS)
		actual["start_delta_ms"] = delta
		if delta > startTolerance {
			check.Pass = false
		}
	} else {
		actual["start"] = "not exposed by both versions"
	}
	if own.DurationAvailable && other.DurationAvailable {
		delta := absInt64(own.DurationMS - other.DurationMS)
		actual["duration_delta_ms"] = delta
		if delta > durationTolerance {
			check.Pass = false
		}
	} else {
		actual["duration"] = "not exposed by both versions"
	}
	check.Actual = actual
	if !check.Pass {
		check.Detail = "default " + kind + " timing differs beyond tolerance"
	}
	return check
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
