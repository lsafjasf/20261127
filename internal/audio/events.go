package audio

import (
	"math"
	"sort"
)

func estimateFrameLags(reference, dialogue []float64, frames []FrameMetric, cfg AnalysisConfig, sampleRate int) []int {
	decimation := max(1, sampleRate/1000)
	referenceDown := decimate(reference, decimation)
	dialogueDown := decimate(dialogue, decimation)
	maxLagDown := max(1, int(math.Ceil(cfg.MaxLagSec*float64(sampleRate)/float64(decimation))))
	lags := make([]int, 0, len(frames))
	for _, fm := range frames {
		if !fm.DialogueActive || fm.LowDBFS[cfg.DialogueChannel] <= cfg.LowFloorDBFS {
			continue
		}
		start := fm.StartSample / decimation
		end := min(fm.EndSample/decimation, len(dialogueDown))
		refDB := frameRMSdB(referenceDown, start, end-start)
		if refDB <= cfg.LowFloorDBFS {
			continue
		}
		lag, corr, ok := bestCorrelation(dialogueDown, referenceDown, start, end, maxLagDown)
		if ok && math.Abs(corr) >= cfg.MinCorrelation {
			lags = append(lags, lag*decimation)
		}
	}
	return lags
}

func decimate(x []float64, factor int) []float64 {
	if factor <= 1 {
		return x
	}
	out := make([]float64, (len(x)+factor-1)/factor)
	for i := range out {
		out[i] = x[min(len(x)-1, i*factor)]
	}
	return out
}

func estimateCandidateLag(reference, dialogue []float64, frames []FrameMetric, cfg AnalysisConfig, sampleRate int) int {
	lags := estimateFrameLags(reference, dialogue, frames, cfg, sampleRate)
	if len(lags) == 0 {
		return 0
	}
	sort.Ints(lags)
	return refineLag(reference, dialogue, frames, cfg, sampleRate, lags[len(lags)/2])
}

type frameRange struct {
	start, end int
	priority   float64
}

func refineLag(reference, dialogue []float64, frames []FrameMetric, cfg AnalysisConfig, sampleRate, initial int) int {
	var ranges []frameRange
	for _, fm := range frames {
		if !fm.DialogueActive || fm.LowDBFS[cfg.DialogueChannel] <= cfg.LowFloorDBFS {
			continue
		}
		refDB := frameRMSdB(reference, fm.StartSample, fm.EndSample-fm.StartSample)
		if refDB <= cfg.LowFloorDBFS {
			continue
		}
		ranges = append(ranges, frameRange{
			start: fm.StartSample, end: fm.EndSample,
			priority: min(refDB, fm.LowDBFS[cfg.DialogueChannel]),
		})
	}
	sort.SliceStable(ranges, func(i, j int) bool { return ranges[i].priority > ranges[j].priority })
	if len(ranges) > 24 {
		ranges = ranges[:24]
	}
	decimation := max(1, sampleRate/1000)
	search := decimation
	lo, hi := max(-int(cfg.MaxLagSec*float64(sampleRate)), initial-search),
		min(int(cfg.MaxLagSec*float64(sampleRate)), initial+search)
	bestLag, bestCorr := initial, 0.0
	for lag := lo; lag <= hi; lag++ {
		var dot, de, re float64
		for _, r := range ranges {
			d, dend, rend, _ := dotAt(dialogue, reference, r.start, r.end, lag)
			dot += d
			de += dend
			re += rend
		}
		if de <= 1e-18 || re <= 1e-18 {
			continue
		}
		c := dot / math.Sqrt(de*re)
		if math.Abs(c) > math.Abs(bestCorr) {
			bestCorr, bestLag = c, lag
		}
	}
	return bestLag
}

func buildEvents(dialogue, reference []float64, frames []FrameMetric, sampleRate int, cfg AnalysisConfig, sourceIndex int, sourceName string) []LeakEvent {
	var groups [][]FrameMetric
	var current []FrameMetric
	mergeSamples := int(math.Round(cfg.MergeGapSec * float64(sampleRate)))
	for _, fm := range frames {
		if !fm.Leak {
			continue
		}
		if len(current) == 0 || fm.StartSample-current[len(current)-1].EndSample <= mergeSamples {
			current = append(current, fm)
			continue
		}
		groups = append(groups, current)
		current = []FrameMetric{fm}
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}

	minSamples := int(math.Round(cfg.MinDurationSec * float64(sampleRate)))
	events := make([]LeakEvent, 0, len(groups))
	for _, group := range groups {
		start := group[0].StartSample
		end := group[len(group)-1].EndSample
		start, end = refineLeakBounds(reference, start, end, sampleRate, cfg.LowFloorDBFS)
		if end-start < minSamples {
			continue
		}
		lag := group[len(group)/2].LagSamples
		corr := correlationAt(dialogue, reference, start, end, lag)
		gain := leastSquaresGain(dialogue, reference, start, end, lag)
		dDB := RMSdBFS(dialogue[start:end])
		rDB := rmsAligned(reference, start, end, lag)
		var residualSum float64
		var count int
		for i := start; i < end; i++ {
			j := i - lag
			if j < 0 || j >= len(reference) {
				continue
			}
			residual := dialogue[i] - gain*reference[j]
			residualSum += residual * residual
			count++
		}
		residualDB := -240.0
		if count > 0 {
			residualDB = 20 * math.Log10(max(math.Sqrt(residualSum/float64(count)), 1e-12))
		}
		event := LeakEvent{
			Index:                 len(events),
			StartSample:           start,
			EndSample:             end,
			StartSec:              float64(start) / float64(sampleRate),
			EndSec:                float64(end) / float64(sampleRate),
			DurationSec:           float64(end-start) / float64(sampleRate),
			SourceIndex:           sourceIndex,
			SourceName:            sourceName,
			Correlation:           corr,
			LagSamples:            lag,
			LagMS:                 float64(lag) * 1000 / float64(sampleRate),
			Gain:                  gain,
			DialogueLowDBFS:       dDB,
			SourceLowDBFS:         rDB,
			ProjectedResidualDBFS: residualDB,
			ProjectedReductionDB:  dDB - residualDB,
		}
		events = append(events, event)
	}
	return events
}

func rmsAligned(reference []float64, start, end, lag int) float64 {
	lo := max(start, lag)
	hi := min(end, len(reference)+lag)
	if hi <= lo {
		return -240
	}
	return RMSdBFS(reference[lo-lag : hi-lag])
}

func refineLeakBounds(reference []float64, start, end, sampleRate int, floorDB float64) (int, int) {
	start = max(0, start)
	end = min(len(reference), end)
	if end-start < 64 {
		return start, end
	}
	envWin := max(16, sampleRate/100)
	hop := max(1, sampleRate/500)
	var envDB []float64
	var centers []int
	peak := math.Inf(-1)
	for pos := start; pos < end; pos += hop {
		db := frameRMSdB(reference, pos, envWin)
		envDB = append(envDB, db)
		centers = append(centers, pos+envWin/2)
		peak = max(peak, db)
	}
	threshold := max(floorDB, peak-35)
	first, last := -1, -1
	for i, db := range envDB {
		if db >= threshold {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return start, end
	}
	refinedStart := max(start, centers[first]-envWin)
	refinedEnd := min(end, centers[last]+envWin/2)
	if refinedEnd <= refinedStart {
		return start, end
	}
	return refinedStart, refinedEnd
}
