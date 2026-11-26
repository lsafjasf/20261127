package analyze

import (
	"clipforensic/internal/wav"
	"fmt"
	"math"
)

func Compare(master, distributed *wav.Audio, masterPath, distributedPath string, opts Options) (Report, error) {
	if master.SampleRate != distributed.SampleRate {
		return Report{}, fmt.Errorf("sample rates differ: master=%d distribution=%d; decode/resample to a common rate first", master.SampleRate, distributed.SampleRate)
	}
	opts = NormalizeOptions(opts, distributed.SampleRate)
	lag := AlignLag(master, distributed)
	fileReport := Analyze(distributed, opts)
	fileReport.File = distributedPath
	masterRuns := findRuns(master, opts)
	distRuns := findRuns(distributed, opts)

	for i := range fileReport.Candidates {
		c := &fileReport.Candidates[i]
		mappedStart := c.StartSample + lag
		mappedEnd := c.EndSample + lag
		refRuns := filterRunsInRange(masterRuns, mappedStart-opts.MergeGapSamples, mappedEnd+opts.MergeGapSamples)
		encRuns := filterRunsInRange(distRuns, c.StartSample, c.EndSample)
		matched := 0
		for _, er := range encRuns {
			if runHasOverlap(er, refRuns, lag, 96) {
				matched++
			}
		}
		longestRef := 0
		for _, r := range refRuns {
			if n := r.End - r.Start + 1; n > longestRef {
				longestRef = n
			}
		}
		verdict := "inconclusive_reference_mismatch"
		if len(refRuns) > 0 && longestRef >= maxInt(2, c.Plateau.LongestRun/2) {
			verdict = "plateau_present_in_master__mastering_or_prefile_limiter"
			c.Classification = "mastering_limiter_clip_reference_confirmed"
			c.Attribution = "对齐后的编码前母带在同一时间已有同尺度平台；编码/平台转码不是削波平台的来源，最多造成边缘轻微变化。"
		} else if len(refRuns) == 0 && c.Plateau.LongestRun >= 3 {
			verdict = "new_plateau_in_distribution__encode_decode_or_transcode_candidate"
			c.Classification = "distribution_encode_or_transcode_new_clip"
			c.Attribution = "编码前母带同位置没有近顶平台，而分发解码版出现新平台；这支持编码、转码、播放器采样转换或母带后增益引入，而不是原始母带硬削波。"
		} else if len(refRuns) > 0 {
			verdict = "master_clip_with_distribution_change"
			c.Attribution = "母带已有部分平台，分发版平台数量或长度明显变化；母带削波和编码/转码边缘改变同时存在。"
		}
		c.Comparison = &ComparisonEvidence{
			ReferenceFile:        masterPath,
			AlignedLagSamples:    lag,
			ReferencePeak:        peakAmplitude(master),
			ReferencePlateauRuns: len(refRuns),
			ReferenceHadPlateau:  len(refRuns) > 0,
			NewPlateauRuns:       len(encRuns) - matched,
			PeakDifferenceDB:     dbFS(peakAmplitude(distributed)) - dbFS(peakAmplitude(master)),
			Verdict:              verdict,
		}
		c.Reasons = append(c.Reasons, fmt.Sprintf("母带对齐 lag=%d samples；参考平台段 %d，新增段约 %d", lag, len(refRuns), len(encRuns)-matched))
	}

	report := Report{SchemaVersion: "1.0", Tool: "clipforensic compare", Files: []FileReport{fileReport}}
	report.Summary.FilesAnalyzed = 1
	report.Summary.IncidentCandidates = len(fileReport.Candidates)
	report.Verdict = "comparison_complete"
	report.Notes = []string{fmt.Sprintf("Aligned distribution to master with lag=%d samples; positive lag means distribution sample i maps to master sample i+lag.", lag)}
	return report, nil
}

func AlignLag(master, distributed *wav.Audio) int {
	m := mono(master)
	d := mono(distributed)
	base := firstOnsetSample(m, master.SampleRate) - firstOnsetSample(d, distributed.SampleRate)
	limit := minInt(len(d), 240000)
	bestLag, bestSampleCorr := base, -2.0
	for lag := base - 192; lag <= base+192; lag++ {
		var dot, dd, mm float64
		n := 0
		for i := 0; i < limit; i += 2 {
			j := i + lag
			if j < 0 || j >= len(m) {
				continue
			}
			dv, mv := d[i], m[j]
			dot += dv * mv
			dd += dv * dv
			mm += mv * mv
			n++
		}
		if n > 100 {
			corr := dot / math.Sqrt(dd*mm+1e-12)
			if corr > bestSampleCorr {
				bestSampleCorr = corr
				bestLag = lag
			}
		}
	}
	return bestLag
}

func firstOnsetSample(x []float64, sampleRate int) int {
	block := maxInt(64, sampleRate/200)
	env := blockEnvelope(x, block)
	peak := 0.0
	for _, v := range env {
		if v > peak {
			peak = v
		}
	}
	threshold := math.Max(0.02, peak*0.18)
	for i, v := range env {
		if v >= threshold {
			return i * block
		}
	}
	return 0
}

func normalizeEnvelope(x []float64) []float64 {
	peak := 0.0
	for _, v := range x {
		if v > peak {
			peak = v
		}
	}
	if peak <= 1e-12 {
		return x
	}
	out := make([]float64, len(x))
	for i, v := range x {
		out[i] = v / peak
	}
	return out
}

func mono(a *wav.Audio) []float64 {
	out := make([]float64, a.Frames)
	for i := range out {
		s := 0.0
		for c := range a.Samples {
			s += a.Samples[c][i]
		}
		out[i] = s / float64(a.Channels)
	}
	return out
}

func blockEnvelope(x []float64, block int) []float64 {
	n := len(x) / block
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := 0.0
		for _, v := range x[i*block : (i+1)*block] {
			sum += v * v
		}
		out[i] = math.Sqrt(sum / float64(block))
	}
	return out
}

func envelopeCorrelation(master, dist []float64, blockLag int) (float64, int) {
	var dot, dd, mm float64
	n := 0
	for di, dv := range dist {
		mi := di + blockLag
		if mi < 0 || mi >= len(master) {
			continue
		}
		mv := master[mi]
		dot += dv * mv
		dd += dv * dv
		mm += mv * mv
		n++
	}
	if n == 0 {
		return -2, 0
	}
	return dot / math.Sqrt(dd*mm+1e-12), n
}

func filterRunsInRange(runs []plateauRun, start, end int) []plateauRun {
	var out []plateauRun
	for _, r := range runs {
		if r.Start <= end && r.End >= start {
			out = append(out, r)
		}
	}
	return out
}

func runHasOverlap(r plateauRun, refs []plateauRun, lag, tolerance int) bool {
	for _, q := range refs {
		mappedStart := r.Start + lag - tolerance
		mappedEnd := r.End + lag + tolerance
		if q.Start <= mappedEnd && q.End >= mappedStart {
			return true
		}
	}
	return false
}

func peakAmplitude(a *wav.Audio) float64 {
	peak := 0.0
	for _, ch := range a.Samples {
		for _, v := range ch {
			if math.Abs(v) > peak {
				peak = math.Abs(v)
			}
		}
	}
	return peak
}
