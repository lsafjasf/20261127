package webm

import (
	"fmt"
	"math"
	"sort"
)

func analyzeAndValidateTiming(rep *Report, opts Options) {
	trackNumber := primaryTrackNumber(rep)
	frames := framesForTrack(rep.Frames, trackNumber)
	var defaultNS uint64
	for i := range rep.Tracks {
		if rep.Tracks[i].TrackNumber == trackNumber {
			defaultNS = rep.Tracks[i].DefaultDurationNS
			break
		}
	}

	timing := TimingReport{}
	if defaultNS > 0 {
		timing.DefaultFrameRate = 1e9 / float64(defaultNS)
	}
	if len(frames) == 0 {
		rep.Timing = timing
		addFinding(rep, Finding{Severity: "error", Layer: "container", Class: "muxing_error", Code: "no_video_frames", Track: trackNumber, Message: "no video Block/SimpleBlock frames were parsed"})
		return
	}

	timing.FirstPTSNS = frames[0].PTSNS
	timing.LastPTSNS = frames[len(frames)-1].PTSNS
	timing.FrameSpanNS = timing.LastPTSNS - timing.FirstPTSNS

	var deltas []int64
	var positive []int64
	var sum int64
	for i := 1; i < len(frames); i++ {
		delta := frames[i].PTSNS - frames[i-1].PTSNS
		deltas = append(deltas, delta)
		sum += delta
		switch {
		case delta < 0:
			timing.NonMonotonic++
		case delta == 0:
			timing.Duplicates++
		default:
			positive = append(positive, delta)
		}
	}
	if len(deltas) > 0 {
		timing.MeanIntervalNS = float64(sum) / float64(len(deltas))
	}
	if len(positive) > 0 {
		timing.MinIntervalNS = positive[0]
		timing.MaxIntervalNS = positive[0]
		var ps []float64
		var positiveSum int64
		for _, d := range positive {
			if d < timing.MinIntervalNS {
				timing.MinIntervalNS = d
			}
			if d > timing.MaxIntervalNS {
				timing.MaxIntervalNS = d
			}
			positiveSum += d
			ps = append(ps, float64(d))
		}
		mean := float64(positiveSum) / float64(len(positive))
		var variance float64
		for _, d := range ps {
			variance += (d - mean) * (d - mean)
		}
		variance /= float64(len(ps))
		if mean > 0 {
			timing.IntervalCV = math.Sqrt(variance) / mean
		}
		if defaultNS == 0 {
			sorted := append([]int64(nil), positive...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			defaultNS = uint64(sorted[len(sorted)/2])
		}
	}

	tol := opts.IntervalTolerance
	if tol <= 0 {
		tol = 0.20
	}
	if defaultNS > 0 {
		for i, delta := range deltas {
			switch {
			case delta < 0:
				addFinding(rep, blockFinding("error", "muxing_error", "non_monotonic_pts", trackNumber, frames[i+1].Index,
					fmt.Sprintf("PTS went backwards by %d ns", -delta)))
			case delta == 0:
				addFinding(rep, blockFinding("warning", "muxing_error", "duplicate_pts", trackNumber, frames[i+1].Index, "adjacent frames share one PTS"))
			default:
				ratio := float64(delta) / float64(defaultNS)
				if ratio > 1+tol {
					missing := int(math.Round(ratio)) - 1
					if missing >= 1 {
						timing.Gaps += missing
						addFinding(rep, blockFinding("error", "muxing_error", "pts_gap", trackNumber, frames[i+1].Index,
							fmt.Sprintf("interval %d ns is %.2fx frame duration; about %d frame slot(s) missing", delta, ratio, missing)))
					} else {
						timing.IrregularIntervals++
					}
				} else if ratio < 1-tol {
					timing.IrregularIntervals++
					addFinding(rep, blockFinding("warning", "muxing_error", "short_pts_interval", trackNumber, frames[i+1].Index,
						fmt.Sprintf("interval %d ns is %.2fx frame duration", delta, ratio)))
				}
			}
		}
	}

	lastEnd := timing.LastPTSNS
	if n := len(frames); n > 0 && frames[n-1].DurationNS > 0 {
		lastEnd += int64(frames[n-1].DurationNS)
	} else if defaultNS > 0 {
		lastEnd += int64(defaultNS)
	}
	if rep.ContainerDurationNS > 0 && defaultNS > 0 {
		delta := rep.ContainerDurationNS - lastEnd
		if delta > int64(2*defaultNS) {
			addFinding(rep, Finding{Severity: "warning", Layer: "container", Class: "muxing_error", Code: "container_longer_than_frames", Track: trackNumber,
				Message: fmt.Sprintf("Info.Duration exceeds last PTS+frame duration by %.3f ms", float64(delta)/1e6)})
		} else if -delta > int64(defaultNS) {
			addFinding(rep, Finding{Severity: "warning", Layer: "container", Class: "muxing_error", Code: "container_shorter_than_frames", Track: trackNumber,
				Message: fmt.Sprintf("last PTS+frame duration exceeds Info.Duration by %.3f ms", float64(-delta)/1e6)})
		}
	}
	rep.Timing = timing
}

func primaryTrackNumber(rep *Report) uint64 {
	for _, track := range rep.Tracks {
		if track.TrackType == 1 {
			return track.TrackNumber
		}
	}
	if len(rep.Tracks) > 0 {
		return rep.Tracks[0].TrackNumber
	}
	if len(rep.Frames) > 0 {
		return rep.Frames[0].TrackNumber
	}
	return 0
}

func framesForTrack(in []FrameReport, track uint64) []FrameReport {
	if track == 0 {
		return in
	}
	var out []FrameReport
	for _, f := range in {
		if f.TrackNumber == track {
			out = append(out, f)
		}
	}
	return out
}
