package manifest

import (
	"fmt"
	"math"
)

func (a *assessor) buildTimeline(hdr, sdr VideoInfo) TimelineReport {
	report := TimelineReport{
		Status: StatusIncomplete,
		HDR:    timelineActual(hdr),
		SDR:    timelineActual(sdr),
		Checks: []Check{},
	}
	for _, item := range []struct {
		role  string
		video VideoInfo
	}{
		{role: "HDR", video: hdr},
		{role: "SDR", video: sdr},
	} {
		report.Checks = append(report.Checks,
			availabilityCheck("timeline."+item.role+".duration_available", "duration", item.video.DurationAvailable),
			availabilityCheck("timeline."+item.role+".start_available", "start time", item.video.StartAvailable),
			availabilityCheck("timeline."+item.role+".frame_rate_available", "frame rate", item.video.FrameRateAvailable),
			availabilityCheck("timeline."+item.role+".frame_count_available", "exact frame count", item.video.FrameCountAvailable),
		)
	}

	durationTolerance := a.durationToleranceMS()
	startTolerance := a.startToleranceMS()
	frameRateTolerance := a.cfg.Timeline.FrameRateTolerance
	if frameRateTolerance <= 0 {
		frameRateTolerance = defaultFrameRateTolerance
	}

	report.Checks = append(report.Checks, deltaInt64Check(
		"timeline.duration_delta",
		"duration difference",
		hdr.DurationAvailable && sdr.DurationAvailable,
		hdr.DurationMS,
		sdr.DurationMS,
		durationTolerance,
		"ms",
	))
	report.Checks = append(report.Checks, deltaInt64Check(
		"timeline.start_delta",
		"start time difference",
		hdr.StartAvailable && sdr.StartAvailable,
		hdr.StartMS,
		sdr.StartMS,
		startTolerance,
		"ms",
	))

	frameAvailable := hdr.FrameRateAvailable && sdr.FrameRateAvailable
	frameCheck := Check{
		ID:       "timeline.frame_rate_delta",
		Category: "timeline",
		Required: true,
		Expected: fmt.Sprintf("difference <= %g fps", frameRateTolerance),
	}
	if !frameAvailable {
		frameCheck.Actual = "frame rate unavailable"
		frameCheck.Detail = "both versions must expose a video frame rate"
	} else {
		delta := math.Abs(hdr.FrameRate - sdr.FrameRate)
		frameCheck.Actual = fmt.Sprintf("%.6f fps", delta)
		frameCheck.Pass = delta <= frameRateTolerance
		if !frameCheck.Pass {
			frameCheck.Detail = "HDR and SDR frame rates differ beyond tolerance"
		}
	}
	report.Checks = append(report.Checks, frameCheck)

	frameCountCheck := Check{
		ID:       "timeline.frame_count_delta",
		Category: "timeline",
		Required: true,
	}
	exactFrames := hdr.FrameCountAvailable && sdr.FrameCountAvailable
	estimatedFrames := hdr.DurationAvailable && sdr.DurationAvailable &&
		hdr.FrameRateAvailable && sdr.FrameRateAvailable &&
		hdr.EstimatedFrames > 0 && sdr.EstimatedFrames > 0
	switch {
	case exactFrames:
		delta := hdr.FrameCount - sdr.FrameCount
		if delta < 0 {
			delta = -delta
		}
		frameCountCheck.Expected = "equal exact frame count"
		frameCountCheck.Actual = map[string]any{
			"hdr":    hdr.FrameCount,
			"sdr":    sdr.FrameCount,
			"delta":  delta,
			"source": hdr.FrameCountSource,
		}
		frameCountCheck.Pass = delta == 0
		if !frameCountCheck.Pass {
			frameCountCheck.Detail = "HDR and SDR exact frame counts differ"
		}
	case estimatedFrames:
		delta := hdr.EstimatedFrames - sdr.EstimatedFrames
		if delta < 0 {
			delta = -delta
		}
		frameCountCheck.Expected = "difference <= 1 estimated frame"
		frameCountCheck.Actual = map[string]any{
			"hdr":    hdr.EstimatedFrames,
			"sdr":    sdr.EstimatedFrames,
			"delta":  delta,
			"source": "duration_x_frame_rate",
		}
		frameCountCheck.Pass = delta <= 1
		if !frameCountCheck.Pass {
			frameCountCheck.Detail = "HDR and SDR timelines differ by more than one estimated frame"
		}
	default:
		frameCountCheck.Expected = "exact or estimated frame count available for both versions"
		frameCountCheck.Actual = "estimated frame count unavailable"
		frameCountCheck.Detail = "duration and frame rate are required to compare frame counts"
	}
	report.Checks = append(report.Checks, frameCountCheck)
	report.Status = statusFromChecks(report.Checks)
	return report
}

func (a *assessor) durationToleranceMS() int64 {
	if a.cfg.Timeline.DurationToleranceMS > 0 {
		return a.cfg.Timeline.DurationToleranceMS
	}
	return defaultDurationToleranceMS
}

func (a *assessor) startToleranceMS() int64 {
	if a.cfg.Timeline.StartToleranceMS > 0 {
		return a.cfg.Timeline.StartToleranceMS
	}
	return defaultStartToleranceMS
}

func timelineActual(video VideoInfo) TimelineActual {
	return TimelineActual{
		DurationMS:          video.DurationMS,
		DurationAvailable:   video.DurationAvailable,
		DurationSource:      video.DurationSource,
		StartMS:             video.StartMS,
		StartAvailable:      video.StartAvailable,
		FrameRate:           video.FrameRate,
		FrameRateAvailable:  video.FrameRateAvailable,
		FrameRateText:       video.FrameRateText,
		FrameCount:          video.FrameCount,
		FrameCountAvailable: video.FrameCountAvailable,
		FrameCountSource:    video.FrameCountSource,
		EstimatedFrames:     video.EstimatedFrames,
	}
}

func availabilityCheck(id, label string, available bool) Check {
	check := Check{
		ID:       id,
		Category: "timeline",
		Required: true,
		Expected: "available",
		Actual:   boolAvailability(available),
		Pass:     available,
	}
	if !available {
		check.Detail = label + " is missing from the video timeline"
	}
	return check
}

func boolAvailability(value bool) string {
	if value {
		return "available"
	}
	return "missing"
}

func deltaInt64Check(id, label string, available bool, hdr, sdr, tolerance int64, unit string) Check {
	check := Check{
		ID:       id,
		Category: "timeline",
		Required: true,
		Expected: fmt.Sprintf("difference <= %d %s", tolerance, unit),
	}
	if !available {
		check.Actual = "unavailable"
		check.Detail = label + " cannot be compared because one version has no value"
		return check
	}
	delta := hdr - sdr
	if delta < 0 {
		delta = -delta
	}
	check.Actual = fmt.Sprintf("%d %s", delta, unit)
	check.Pass = delta <= tolerance
	if !check.Pass {
		check.Detail = fmt.Sprintf("%s exceeds %d %s tolerance", label, tolerance, unit)
	}
	return check
}
