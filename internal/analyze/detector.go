package analyze

import (
	"clipforensic/internal/wav"
	"fmt"
	"math"
	"sort"
)

type plateauRun struct {
	Channel int
	Start   int
	End     int
	Level   float64
	ExactFS bool
}

type region struct {
	Start int
	End   int
	Runs  []plateauRun
}

const exactFSLimit = 0.99994

func Analyze(a *wav.Audio, opts Options) FileReport {
	opts = NormalizeOptions(opts, a.SampleRate)
	report := FileReport{
		SampleRate:  a.SampleRate,
		Channels:    a.Channels,
		Frames:      a.Frames,
		DurationSec: float64(a.Frames) / float64(a.SampleRate),
	}
	report.Global = globalMetrics(a)
	runs := findRuns(a, opts)
	report.Global.PlateauRunCount = len(runs)
	for _, r := range runs {
		n := r.End - r.Start + 1
		report.Global.PlateauSamples += n
		if n > report.Global.LongestPlateauRun {
			report.Global.LongestPlateauRun = n
		}
	}
	regions := groupRuns(runs, opts.MergeGapSamples)

	for i, reg := range regions {
		cand := buildCandidate(a, opts, reg, runs)
		cand.ID = fmt.Sprintf("C%03d", i+1)
		scoreCandidate(&cand)
		if cand.Confidence >= opts.MinConfidence && cand.Plateau.LongestRun >= opts.MinPlateauRun {
			cand.Status = "incident"
			cand.Incident = true
			cand.Severity = severity(cand.Confidence)
			report.Candidates = append(report.Candidates, cand)
		} else {
			cand.Status = "inconclusive"
			cand.Classification = "inconclusive_clip_like"
			report.Artistic = append(report.Artistic, cand)
		}
	}

	art := findArtisticRegions(a, opts, report.Candidates)
	report.Artistic = append(report.Artistic, art...)
	for i := range report.Artistic {
		if report.Artistic[i].ID == "" {
			report.Artistic[i].ID = fmt.Sprintf("A%03d", i+1)
		}
	}

	if len(report.Candidates) > 0 {
		report.Verdict = "clip_incidents_detected"
	} else if len(report.Artistic) > 0 {
		report.Verdict = "artistic_distortion_without_digital_clip_incident"
	} else {
		report.Verdict = "no_clip_incident_detected"
	}
	return report
}

func globalMetrics(a *wav.Audio) GlobalMetrics {
	m := GlobalMetrics{}
	sum := 0.0
	n := 0
	for _, ch := range a.Samples {
		for _, v := range ch {
			av := math.Abs(v)
			if av > m.PeakAmplitude {
				m.PeakAmplitude = av
			}
			if av >= exactFSLimit {
				m.ExactFullScaleSamples++
			}
			if av >= 0.98 {
				m.NearCeilingSamples++
			}
			sum += v
			n++
		}
	}
	if n > 0 {
		m.DCOffset = sum / float64(n)
	}
	m.PeakDBFS = dbFS(m.PeakAmplitude)
	return m
}

func findRuns(a *wav.Audio, opts Options) []plateauRun {
	var runs []plateauRun
	for c, ch := range a.Samples {
		i := 0
		for i < len(ch) {
			if math.Abs(ch[i]) < opts.CeilingThreshold {
				i++
				continue
			}
			start := i
			end := i
			sumLevel := math.Abs(ch[i])
			for j := i + 1; j < len(ch); j++ {
				if math.Abs(ch[j]) < opts.CeilingThreshold {
					break
				}
				if math.Abs(ch[j]-ch[j-1]) > opts.FlatTolerance {
					break
				}
				end = j
				sumLevel += math.Abs(ch[j])
			}
			length := end - start + 1
			if length >= opts.MinPlateauRun {
				exact := false
				for _, v := range ch[start : end+1] {
					if math.Abs(v) >= exactFSLimit {
						exact = true
						break
					}
				}
				runs = append(runs, plateauRun{
					Channel: c, Start: start, End: end,
					Level: sumLevel / float64(length), ExactFS: exact,
				})
			}
			i = end + 1
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Start == runs[j].Start {
			return runs[i].Channel < runs[j].Channel
		}
		return runs[i].Start < runs[j].Start
	})
	return runs
}

func groupRuns(runs []plateauRun, gap int) []region {
	var groups []region
	for _, r := range runs {
		if len(groups) == 0 || r.Start > groups[len(groups)-1].End+gap {
			groups = append(groups, region{Start: r.Start, End: r.End, Runs: []plateauRun{r}})
			continue
		}
		g := &groups[len(groups)-1]
		g.Runs = append(g.Runs, r)
		if r.End > g.End {
			g.End = r.End
		}
	}
	return groups
}

func buildCandidate(a *wav.Audio, opts Options, reg region, allRuns []plateauRun) Candidate {
	cand := Candidate{
		StartSample: reg.Start,
		EndSample:   reg.End,
		StartSec:    float64(reg.Start) / float64(a.SampleRate),
		EndSec:      float64(reg.End) / float64(a.SampleRate),
		Channels:    uniqueChannels(reg.Runs),
		Reasons:     []string{},
	}
	cand.Channel = primaryChannel(reg.Runs)
	ch := a.Samples[cand.Channel]

	peak := reg.Start
	for i := reg.Start; i <= reg.End && i < len(ch); i++ {
		if math.Abs(ch[i]) > math.Abs(ch[peak]) {
			peak = i
		}
	}
	cand.PeakSample = peak

	longest := reg.Runs[0]
	for _, r := range reg.Runs {
		if r.End-r.Start > longest.End-longest.Start ||
			(r.End-r.Start == longest.End-longest.Start && r.Channel == cand.Channel) {
			longest = r
		}
	}
	cand.EvidenceSample = longest.Start
	cand.Plateau = plateauMetrics(reg, len(cand.Channels))
	cand.Edge = edgeMetrics(ch, reg.Runs, longest, opts)

	specLo := clampIndex(reg.Start-opts.FFTSize/4, len(ch))
	specHi := clampIndex(reg.End+opts.FFTSize/4, len(ch))
	if specHi-specLo < 256 {
		specLo = clampIndex(cand.EvidenceSample-opts.FFTSize/2, len(ch))
		specHi = clampIndex(cand.EvidenceSample+opts.FFTSize/2, len(ch))
	}
	cand.Spectrum = analyzeSpectrum(ch[specLo:specHi], a.SampleRate, maxInt(opts.FFTSize, 16384))
	cand.Spectrum.SplatterDeltaDB = splatterDeltaDB(ch, reg, cand.EvidenceSample, a.SampleRate)
	cand.Waveform = waveformEvidence(a, cand.EvidenceSample, opts, reg.Runs)
	cand.Classification = "mastering_limiter_clip"
	cand.Attribution = "连续近满刻度平台、陡直沿和宽带谐波符合母带/限幅器硬削波；最终归因需与编码前母带对齐复核。"
	return cand
}

func plateauMetrics(reg region, channelCount int) PlateauMetrics {
	m := PlateauMetrics{RunCount: len(reg.Runs)}
	lengthSum := 0
	for _, r := range reg.Runs {
		n := r.End - r.Start + 1
		m.PlateauSamples += n
		lengthSum += n
		if n > m.LongestRun {
			m.LongestRun = n
		}
		if r.ExactFS {
			m.ExactFullScaleSamples += n
		}
		for i := r.Start; i <= r.End; i++ {
			_ = i
			m.NearCeilingSamples++
		}
	}
	if m.RunCount > 0 {
		m.MeanRun = float64(lengthSum) / float64(m.RunCount)
	}
	span := reg.End - reg.Start + 1
	if span > 0 && channelCount > 0 {
		m.Density = float64(m.PlateauSamples) / float64(span*channelCount)
	}
	return m
}

func edgeMetrics(ch []float64, runs []plateauRun, primary plateauRun, opts Options) EdgeMetrics {
	m := EdgeMetrics{MedianRiseSamples: math.NaN()}
	var rises []float64
	var corners []float64
	for _, r := range runs {
		if r.Channel != primary.Channel {
			continue
		}
		level := math.Max(0.1, r.Level)
		if r.Start > 0 {
			jump := math.Abs(ch[r.Start] - ch[r.Start-1])
			if jump > m.MaxEntryJump {
				m.MaxEntryJump = jump
			}
			slope := localMaxAbsDiff(ch, r.Start-4, r.Start+2)
			if slope > m.MaxSlope {
				m.MaxSlope = slope
			}
		}
		if r.End+1 < len(ch) {
			jump := math.Abs(ch[r.End+1] - ch[r.End])
			if jump > m.MaxExitJump {
				m.MaxExitJump = jump
			}
			slope := localMaxAbsDiff(ch, r.End-1, r.End+5)
			if slope > m.MaxSlope {
				m.MaxSlope = slope
			}
		}
		if rise := riseTime(ch, r.Start, level); rise >= 0 {
			rises = append(rises, float64(rise))
		}
		if corner := cornerTime(ch, r.Start, level); corner >= 0 {
			corners = append(corners, float64(corner))
			if corner <= 2 {
				m.SteepEdgeRatio++
			}
		}
		contextMax := 0.0
		lo := maxInt(0, r.Start-8)
		hi := minInt(len(ch), r.End+9)
		for i := lo; i < hi; i++ {
			if math.Abs(ch[i]) > contextMax {
				contextMax = math.Abs(ch[i])
			}
		}
		if over := contextMax - level; over > m.OvershootAmplitude {
			m.OvershootAmplitude = over
		}
	}
	if count := int(m.SteepEdgeRatio); len(corners) > 0 {
		m.SteepEdgeRatio = float64(count) / float64(len(corners))
		sort.Float64s(rises)
		if len(rises) > 0 {
			m.MedianRiseSamples = rises[len(rises)/2]
		}
	} else {
		m.SteepEdgeRatio = 0
		m.MedianRiseSamples = 0
	}
	if len(corners) > 0 {
		sort.Float64s(corners)
		m.MedianCornerSamples = corners[len(corners)/2]
	}
	return m
}

func riseTime(ch []float64, start int, level float64) int {
	lo := maxInt(0, start-32)
	lowCross := -1
	highCross := start
	for i := lo; i < start; i++ {
		v := math.Abs(ch[i])
		if v < 0.25*level {
			lowCross = i
		}
		if lowCross >= 0 && v >= 0.8*level {
			highCross = i
			break
		}
	}
	if lowCross < 0 {
		return 0
	}
	return highCross - lowCross
}

func cornerTime(ch []float64, start int, level float64) int {
	lo := maxInt(0, start-16)
	below := -1
	for i := lo; i < start; i++ {
		if math.Abs(ch[i]) < 0.90*level {
			below = i
		}
	}
	if below < 0 {
		return 0
	}
	return start - below
}

func localMaxAbsDiff(ch []float64, lo, hi int) float64 {
	lo = maxInt(0, lo)
	hi = minInt(len(ch)-1, hi)
	best := 0.0
	for i := lo; i < hi; i++ {
		if d := math.Abs(ch[i+1] - ch[i]); d > best {
			best = d
		}
	}
	return best
}

func waveformEvidence(a *wav.Audio, center int, opts Options, runs []plateauRun) WaveformEvidence {
	first := maxInt(0, center-opts.WaveBefore)
	last := minInt(a.Frames-1, center+opts.WaveAfter)
	w := WaveformEvidence{
		FirstSample: first, LastSample: last, EvidenceSample: center,
		SampleRate:  a.SampleRate,
		Channels:    make([][]float64, a.Channels),
		PlateauMask: make([][]bool, a.Channels),
	}
	n := last - first + 1
	for c := 0; c < a.Channels; c++ {
		w.Channels[c] = append([]float64(nil), a.Samples[c][first:last+1]...)
		w.PlateauMask[c] = make([]bool, n)
	}
	for _, r := range runs {
		lo := maxInt(first, r.Start)
		hi := minInt(last, r.End)
		for i := lo; i <= hi; i++ {
			w.PlateauMask[r.Channel][i-first] = true
		}
	}
	return w
}

func scoreCandidate(c *Candidate) {
	score := 0.0
	p := c.Plateau
	switch {
	case p.LongestRun >= 8:
		score += 0.22
	case p.LongestRun >= 3:
		score += 0.12
	case p.LongestRun >= 2:
		score += 0.06
	}
	switch {
	case p.RunCount >= 10:
		score += 0.18
	case p.RunCount >= 3:
		score += 0.11
	case p.RunCount >= 1:
		score += 0.05
	}
	if p.ExactFullScaleSamples >= 4 {
		score += 0.15
	} else if p.ExactFullScaleSamples > 0 {
		score += 0.07
	}
	switch {
	case c.Edge.SteepEdgeRatio >= 0.6:
		score += 0.20
	case c.Edge.SteepEdgeRatio >= 0.3:
		score += 0.11
	}
	if p.Density >= 0.08 {
		score += 0.10
	} else if p.Density >= 0.02 {
		score += 0.05
	}
	if c.Spectrum.SplatterDeltaDB >= 14 {
		score += 0.10
	} else if c.Spectrum.SplatterDeltaDB >= 7 {
		score += 0.05
	} else if c.Spectrum.HighRatio+c.Spectrum.AirRatio >= 0.12 {
		score += 0.05
	}
	c.Confidence = clamp01(0.25 + 0.75*score)

	if p.ExactFullScaleSamples > 0 {
		c.Reasons = append(c.Reasons, fmt.Sprintf("发现 %d 个精确满刻度平台样本", p.ExactFullScaleSamples))
	}
	c.Reasons = append(c.Reasons, fmt.Sprintf("最长平台 %d samples，平台段 %d 组", p.LongestRun, p.RunCount))
	if c.Edge.MedianRiseSamples == 0 || !math.IsNaN(c.Edge.MedianRiseSamples) {
		c.Reasons = append(c.Reasons, fmt.Sprintf("上升沿中位 %g samples，陡沿比例 %.2f", c.Edge.MedianRiseSamples, c.Edge.SteepEdgeRatio))
	}
	c.Reasons = append(c.Reasons, fmt.Sprintf("频谱质心 %.0f Hz，4k+ 能量 %.3f%%，邻段高频扩散 %+.1f dB，谐波强度 %.2f",
		c.Spectrum.CentroidHz, 100*(c.Spectrum.HighRatio+c.Spectrum.AirRatio),
		c.Spectrum.SplatterDeltaDB, c.Spectrum.HarmonicStrength))
}

func severity(conf float64) string {
	if conf >= 0.85 {
		return "high"
	}
	if conf >= 0.65 {
		return "medium"
	}
	return "low"
}

func uniqueChannels(runs []plateauRun) []int {
	seen := map[int]bool{}
	var out []int
	for _, r := range runs {
		if !seen[r.Channel] {
			seen[r.Channel] = true
			out = append(out, r.Channel)
		}
	}
	sort.Ints(out)
	return out
}

func primaryChannel(runs []plateauRun) int {
	bestCh := 0
	bestSamples := -1
	byCh := map[int]int{}
	for _, r := range runs {
		byCh[r.Channel] += r.End - r.Start + 1
	}
	for ch, n := range byCh {
		if n > bestSamples {
			bestSamples = n
			bestCh = ch
		}
	}
	return bestCh
}

func clampIndex(v, n int) int {
	if v < 0 {
		return 0
	}
	if v > n {
		return n
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
