package analyze

import (
	"clipforensic/internal/wav"
	"fmt"
	"math"
)

func findArtisticRegions(a *wav.Audio, opts Options, incidents []Candidate) []Candidate {
	mix := make([]float64, a.Frames)
	for f := 0; f < a.Frames; f++ {
		sum := 0.0
		for c := 0; c < a.Channels; c++ {
			sum += a.Samples[c][f]
		}
		mix[f] = sum / float64(a.Channels)
	}

	hop := a.SampleRate / 200
	win := a.SampleRate / 50
	if hop < 1 {
		hop = 1
	}
	if win < hop {
		win = 2 * hop
	}
	env := rmsEnvelope(mix, win, hop)
	peakEnv := 0.0
	for _, v := range env {
		if v > peakEnv {
			peakEnv = v
		}
	}
	activeLevel := math.Max(0.025, peakEnv*0.16)

	var out []Candidate
	allRuns := findRuns(a, opts)
	for _, group := range activeGroups(env, activeLevel, hop, win, a.Frames, a.SampleRate/10) {
		onsets := detectOnsets(env, group.envStart, group.envEnd, hop, a.SampleRate)
		if len(onsets) == 0 {
			onsets = []int{group.start}
		}
		for i, onset := range onsets {
			end := group.end
			if i+1 < len(onsets) {
				end = onsets[i+1]
			}
			start := onset
			if end-start < a.SampleRate/12 {
				continue
			}
			if overlapsIncident(start, end, incidents) {
				continue
			}
			cand := artisticCandidate(a, opts, start, end, allRuns, len(onsets))
			if cand != nil {
				out = append(out, *cand)
			}
		}
	}
	return out
}

type activeGroup struct {
	envStart int
	envEnd   int
	start    int
	end      int
}

func rmsEnvelope(x []float64, win, hop int) []float64 {
	n := (len(x)-win)/hop + 1
	if n < 1 {
		n = 1
	}
	env := make([]float64, n)
	for i := 0; i < n; i++ {
		lo := i * hop
		hi := minInt(len(x), lo+win)
		sum := 0.0
		for _, v := range x[lo:hi] {
			sum += v * v
		}
		env[i] = math.Sqrt(sum / float64(maxInt(1, hi-lo)))
	}
	return env
}

func activeGroups(env []float64, level float64, hop, win, frames, gapSamples int) []activeGroup {
	var groups []activeGroup
	in := false
	start, last := 0, 0
	gapWindows := maxInt(1, gapSamples/hop)
	for i, v := range env {
		if v >= level && !in {
			in = true
			start = i
		}
		if v >= level {
			last = i
		}
		if in && (v < level || i == len(env)-1) && i-last > gapWindows {
			endEnv := last
			groups = append(groups, activeGroup{
				envStart: start,
				envEnd:   endEnv,
				start:    maxInt(0, start*hop-win/2),
				end:      minInt(frames, endEnv*hop+win+gapSamples/2),
			})
			in = false
		}
	}
	if in {
		groups = append(groups, activeGroup{
			envStart: start, envEnd: len(env) - 1,
			start: maxInt(0, start*hop-win/2), end: frames,
		})
	}
	return groups
}

func detectOnsets(env []float64, lo, hi, hop, sampleRate int) []int {
	if hi >= len(env) {
		hi = len(env) - 1
	}
	refractory := maxInt(1, (sampleRate/8)/hop)
	var onsets []int
	lastOnset := -refractory
	for i := maxInt(1, lo); i <= hi; i++ {
		prev := 0.0
		count := 0
		for j := maxInt(lo, i-4); j < i; j++ {
			prev += env[j]
			count++
		}
		if count > 0 {
			prev /= float64(count)
		}
		if env[i] > 0.045 && env[i] > prev*1.35+0.012 && i-lastOnset >= refractory {
			onsets = append(onsets, i*hop)
			lastOnset = i
		}
	}
	return onsets
}

func overlapsIncident(start, end int, incidents []Candidate) bool {
	for _, c := range incidents {
		if start <= c.EndSample && end >= c.StartSample {
			return true
		}
	}
	return false
}

func artisticCandidate(a *wav.Audio, opts Options, start, end int, runs []plateauRun, phraseOnsets int) *Candidate {
	ch := 0
	bestPeak := 0.0
	for c, data := range a.Samples {
		for i := start; i < end; i++ {
			if v := math.Abs(data[i]); v > bestPeak {
				bestPeak = v
				ch = c
			}
		}
	}
	data := a.Samples[ch]
	peak := start
	for i := start; i < end; i++ {
		if math.Abs(data[i]) > math.Abs(data[peak]) {
			peak = i
		}
	}
	noteRuns := filterRuns(runs, start, end, ch)
	reg := region{Start: start, End: end, Runs: noteRuns}
	plateau := plateauMetrics(reg, 1)
	specLo := clampIndex(start, len(data))
	specHi := clampIndex(minInt(end, start+4096), len(data))
	spectrum := analyzeSpectrum(data[specLo:specHi], a.SampleRate, 4096)

	distortionEnergy := spectrum.HighRatio + spectrum.AirRatio
	looksTonal := spectrum.EstimatedF0Hz >= 55 && spectrum.HarmonicStrength >= 0.34
	looksDistorted := distortionEnergy >= 0.10 || spectrum.OddEvenRatio >= 1.35
	noDigitalCeiling := plateau.ExactFullScaleSamples == 0 && plateau.LongestRun <= 2
	if !looksTonal || !looksDistorted || !noDigitalCeiling {
		return nil
	}

	waveOpts := opts
	waveOpts.WaveBefore = a.SampleRate / 100
	waveOpts.WaveAfter = a.SampleRate * 2 / 100
	evidenceCenter := minInt(end-1, start+a.SampleRate/100)
	cand := &Candidate{
		Status:         "artistic_distortion",
		Incident:       false,
		Classification: "artistic_distorted_guitar",
		Attribution:    "起音后保持可估计基频与谐波序列，缺少精确满刻度长平台；高频/奇次谐波来自吉他失真音色，不是母带事故。",
		StartSample:    start,
		EndSample:      end - 1,
		StartSec:       float64(start) / float64(a.SampleRate),
		EndSec:         float64(end-1) / float64(a.SampleRate),
		PeakSample:     peak,
		EvidenceSample: evidenceCenter,
		Channel:        ch,
		Channels:       []int{ch},
		Confidence:     clamp01(0.62 + 0.2*spectrum.HarmonicStrength + 0.1*math.Min(1, distortionEnergy/0.3)),
		Severity:       "none",
		Reasons: []string{
			fmt.Sprintf("检测到音头/拨弦上升沿，乐句内约 %d 个起音", phraseOnsets),
			fmt.Sprintf("稳定基频 %.1f Hz，谐波强度 %.2f", spectrum.EstimatedF0Hz, spectrum.HarmonicStrength),
			fmt.Sprintf("4k+ 能量 %.1f%%，奇/偶谐波比 %.2f，符合失真音色", 100*distortionEnergy, spectrum.OddEvenRatio),
			fmt.Sprintf("精确满刻度平台 %d，最长近顶平台 %d samples", plateau.ExactFullScaleSamples, plateau.LongestRun),
		},
		Plateau:  plateau,
		Spectrum: spectrum,
		Waveform: waveformEvidence(a, evidenceCenter, waveOpts, noteRuns),
	}
	cand.Edge = edgeMetrics(data, noteRuns, primaryOrZero(noteRuns, ch), opts)
	return cand
}

func filterRuns(runs []plateauRun, start, end, ch int) []plateauRun {
	var out []plateauRun
	for _, r := range runs {
		if r.Channel == ch && r.Start <= end && r.End >= start {
			out = append(out, r)
		}
	}
	return out
}

func primaryOrZero(runs []plateauRun, ch int) plateauRun {
	for _, r := range runs {
		if r.Channel == ch {
			return r
		}
	}
	return plateauRun{Channel: ch, Start: 0, End: -1}
}
