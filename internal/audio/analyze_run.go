package audio

import (
	"fmt"
	"math"
	"sort"
)

// Analyze extracts the low band of every channel, gates center dialogue from
// the speech band, and ranks non-dialogue channels by temporal correlation.
func Analyze(b *Buffer, cfg AnalysisConfig) (*Analysis, error) {
	if b == nil || len(b.Samples) == 0 {
		return nil, fmt.Errorf("empty audio buffer")
	}
	f := b.Format
	if f.SampleRate <= 0 || f.Channels <= 0 {
		return nil, fmt.Errorf("invalid WAV format")
	}
	frames := len(b.Samples[0])
	for i, ch := range b.Samples {
		if len(ch) != frames {
			return nil, fmt.Errorf("channel %d length %d differs from %d", i, len(ch), frames)
		}
	}
	if frames == 0 {
		return nil, fmt.Errorf("audio has no samples")
	}
	cfg = cfg.normalized(f.SampleRate)
	names := ChannelNames(f)
	if cfg.DialogueChannel < 0 {
		idx, err := ChannelIndex(f, "FC")
		if err != nil {
			return nil, fmt.Errorf("dialogue channel not specified and no FC channel exists")
		}
		cfg.DialogueChannel = idx
	}
	if cfg.DialogueChannel >= f.Channels {
		return nil, fmt.Errorf("dialogue channel %d out of range", cfg.DialogueChannel)
	}
	if cfg.SourceChannel >= f.Channels {
		return nil, fmt.Errorf("source channel %d out of range", cfg.SourceChannel)
	}
	if cfg.SourceChannel == cfg.DialogueChannel {
		return nil, fmt.Errorf("source and dialogue channels must differ")
	}

	lowBands := make([][]float64, f.Channels)
	for ch := range b.Samples {
		lowBands[ch] = BandPassZeroPhase(b.Samples[ch], f.SampleRate, cfg.LowCutoffHz, cfg.HighCutoffHz)
	}
	dialogueLow := lowBands[cfg.DialogueChannel]
	dialogueBand := BandPassZeroPhase(b.Samples[cfg.DialogueChannel], f.SampleRate, cfg.DialogueLowHz, cfg.DialogueHighHz)

	win := max(64, int(math.Round(cfg.WindowSec*float64(f.SampleRate))))
	hop := max(1, int(math.Round(cfg.HopSec*float64(f.SampleRate))))
	frameList := make([]FrameMetric, 0, frames/hop+1)
	for start := 0; start < frames; start += hop {
		end := min(frames, start+win)
		fm := FrameMetric{
			Index:          len(frameList),
			StartSample:    start,
			EndSample:      end,
			StartSec:       float64(start) / float64(f.SampleRate),
			EndSec:         float64(end) / float64(f.SampleRate),
			DialogueBandDB: frameRMSdB(dialogueBand, start, win),
			LowDBFS:        make([]float64, f.Channels),
		}
		for ch := range lowBands {
			fm.LowDBFS[ch] = frameRMSdB(lowBands[ch], start, win)
		}
		frameList = append(frameList, fm)
		if end == frames {
			break
		}
	}

	gate := dialogueGate(frameList, cfg.DialogueGateDB)
	activeFrames := 0
	for i := range frameList {
		frameList[i].DialogueActive = frameList[i].DialogueBandDB >= gate
		if frameList[i].DialogueActive {
			activeFrames++
		}
	}

	candidates := rankSourceCandidates(lowBands, dialogueLow, frameList, cfg, f.SampleRate)
	sourceIndex := cfg.SourceChannel
	sourceReason := "selected by --source"
	if sourceIndex < 0 {
		if len(candidates) == 0 {
			return nil, fmt.Errorf("no eligible source channel")
		}
		sourceIndex = candidates[0].Index
		sourceReason = "highest low-band temporal correlation with dialogue channel"
	}
	sourceLow := lowBands[sourceIndex]
	sourceName := names[sourceIndex]
	var selectedLag int
	for i := range candidates {
		candidates[i].Selected = candidates[i].Index == sourceIndex
		if candidates[i].Selected {
			selectedLag = candidates[i].LagSamples
		}
	}
	if cfg.SourceChannel >= 0 {
		found := false
		for _, c := range candidates {
			if c.Index == sourceIndex {
				selectedLag = c.LagSamples
				found = true
			}
		}
		if !found {
			selectedLag = estimateCandidateLag(lowBands[sourceIndex], dialogueLow, frameList, cfg, f.SampleRate)
		}
	}

	for i := range frameList {
		frameList[i].SourceLowDBFS = frameList[i].LowDBFS[sourceIndex]
		frameList[i].LagSamples = selectedLag
		if frameList[i].DialogueActive {
			frameList[i].Correlation = correlationAt(
				dialogueLow, sourceLow, frameList[i].StartSample, frameList[i].EndSample, selectedLag)
		}
		frameList[i].Leak = frameList[i].DialogueActive &&
			frameList[i].SourceLowDBFS > cfg.LowFloorDBFS &&
			frameList[i].LowDBFS[cfg.DialogueChannel] >
				leakVisibilityFloor(cfg, frameList[i].DialogueBandDB)+cfg.LeakMarginDB &&
			math.Abs(frameList[i].Correlation) >= cfg.MinCorrelation
	}

	events := buildEvents(dialogueLow, sourceLow, frameList, f.SampleRate, cfg, sourceIndex, sourceName)
	report := Report{
		SampleRate:        f.SampleRate,
		Channels:          f.Channels,
		ChannelNames:      names,
		DurationSec:       float64(frames) / float64(f.SampleRate),
		LowBandHz:         [2]float64{cfg.LowCutoffHz, cfg.HighCutoffHz},
		DialogueChannel:   cfg.DialogueChannel,
		DialogueName:      names[cfg.DialogueChannel],
		DialogueGateDB:    gate,
		DialogueActiveSec: float64(activeFrames) * cfg.HopSec,
		ChannelsSummary:   summarizeChannels(lowBands, names, frameList),
		SourceIndex:       sourceIndex,
		SourceName:        sourceName,
		SourceReason:      sourceReason,
		Candidates:        candidates,
		Events:            events,
	}
	for _, e := range events {
		report.LeakTotalSec += e.DurationSec
	}
	return &Analysis{
		Report:       report,
		Config:       cfg,
		Frames:       frameList,
		DialogueLow:  dialogueLow,
		SourceLow:    sourceLow,
		DialogueBand: dialogueBand,
	}, nil
}

func dialogueGate(frames []FrameMetric, absolute float64) float64 {
	values := make([]float64, len(frames))
	for i, f := range frames {
		values[i] = f.DialogueBandDB
	}
	noise := percentile(values, 0.20)
	peak := percentile(values, 0.95)
	return max(absolute, noise+12, peak-35)
}

func leakVisibilityFloor(cfg AnalysisConfig, dialogueBandDB float64) float64 {
	return max(cfg.LowFloorDBFS, dialogueBandDB-cfg.MaxLeakBelowDialogueDB)
}

func summarizeChannels(lowBands [][]float64, names []string, frames []FrameMetric) []ChannelSummary {
	out := make([]ChannelSummary, len(lowBands))
	for ch := range lowBands {
		out[ch] = ChannelSummary{Index: ch, Name: names[ch], LowEnergyDBFS: RMSdBFS(lowBands[ch])}
		peak := math.Inf(-1)
		var activeEnergy float64
		var activeFrames int
		for _, fm := range frames {
			db := fm.LowDBFS[ch]
			if db > peak {
				peak = db
				out[ch].PeakLowTimeSec = (fm.StartSec + fm.EndSec) / 2
			}
			if fm.DialogueActive {
				activeEnergy += math.Pow(10, db/10)
				activeFrames++
			}
		}
		out[ch].PeakLowFrameDBFS = peak
		if activeFrames > 0 {
			out[ch].DialogueActiveDB = 10 * math.Log10(activeEnergy/float64(activeFrames))
		}
	}
	return out
}

func rankSourceCandidates(lowBands [][]float64, dialogueLow []float64, frames []FrameMetric, cfg AnalysisConfig, sampleRate int) []SourceCandidate {
	names := ChannelNames(Format{Channels: len(lowBands), ChannelMask: DefaultChannelMask(len(lowBands))})
	out := make([]SourceCandidate, 0, len(lowBands)-1)
	for ch := range lowBands {
		if ch == cfg.DialogueChannel {
			continue
		}
		lagVals := estimateFrameLags(lowBands[ch], dialogueLow, frames, cfg, sampleRate)
		lag := 0
		if len(lagVals) > 0 {
			sort.Ints(lagVals)
			lag = lagVals[len(lagVals)/2]
		}
		lag = refineLag(lowBands[ch], dialogueLow, frames, cfg, sampleRate, lag)
		var corrs []float64
		strong := 0
		for _, fm := range frames {
			if !fm.DialogueActive || fm.LowDBFS[ch] <= cfg.LowFloorDBFS ||
				fm.LowDBFS[cfg.DialogueChannel] <= cfg.LowFloorDBFS {
				continue
			}
			c := correlationAt(dialogueLow, lowBands[ch], fm.StartSample, fm.EndSample, lag)
			corrs = append(corrs, math.Abs(c))
			if math.Abs(c) >= cfg.MinCorrelation {
				strong++
			}
		}
		candidate := SourceCandidate{Index: ch, Name: names[ch], LagSamples: lag, LagMS: float64(lag) * 1000 / float64(sampleRate)}
		if len(corrs) > 0 {
			var sum float64
			for _, c := range corrs {
				sum += c
			}
			candidate.MeanAbsCorr = sum / float64(len(corrs))
			candidate.P75AbsCorr = percentile(corrs, 0.75)
			candidate.P90AbsCorr = percentile(corrs, 0.90)
			candidate.StrongFraction = float64(strong) / float64(len(corrs))
			candidate.Score = (0.55*candidate.P75AbsCorr + 0.30*candidate.P90AbsCorr + 0.15*candidate.MeanAbsCorr) *
				(0.5 + 0.5*candidate.StrongFraction)
		}
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Name < out[j].Name
		}
		return out[i].Score > out[j].Score
	})
	return out
}
