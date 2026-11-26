package click

import (
	"math"
	"sort"

	"clickdetect/internal/wav"
)

const (
	blockSeconds    = 0.010
	contextSeconds  = 0.020
	mergeSeconds    = 0.00075
	candidateGapSec = 0.00025
)

type blockProfile struct {
	median float64
	p99    float64
}

type candidate struct {
	sample  int
	lowFreq bool
}

type channelData struct {
	index      int
	x          []float64
	jump       []float64
	slope      []float64
	lowPass    []float64
	lfStep     []float64
	jumpProf   []blockProfile
	slopeProf  []blockProfile
	lfStepProf []blockProfile
}

// Analyze runs click detection over decoded audio.
func Analyze(a *wav.Audio, opts Options) Analysis {
	if opts.WaveformBefore < 0 {
		opts.WaveformBefore = 0
	}
	if opts.WaveformAfter < 0 {
		opts.WaveformAfter = 0
	}
	if opts.MinConfidence <= 0 {
		opts.MinConfidence = DefaultOptions().MinConfidence
	}

	out := Analysis{
		SampleRate:  a.SampleRate,
		Channels:    a.Channels,
		Frames:      a.Frames,
		DurationSec: float64(a.Frames) / float64(a.SampleRate),
		Detections:  []Detection{},
		Suppressed:  []Suppressed{},
	}
	if a.Frames == 0 || a.Channels == 0 {
		return out
	}

	var found []Detection
	var rejected []Suppressed
	for chIndex := 0; chIndex < a.Channels; chIndex++ {
		ch := prepareChannel(a, chIndex)
		cands := scanCandidates(ch, a.SampleRate)
		for _, c := range cands {
			ev := evaluateCandidate(ch, c, a.SampleRate)
			eventSample := ev.sample
			if eventSample == 0 {
				eventSample = c.sample
			}
			if ev.suppressed {
				rejected = append(rejected, Suppressed{
					Sample: eventSample, TimeSec: float64(eventSample) / float64(a.SampleRate),
					Channels: []int{ch.index}, Reason: ev.reason, Features: ev.features,
				})
				continue
			}
			if ev.confidence < opts.MinConfidence {
				continue
			}
			flags := flagsFor(ev.features)
			if len(flags) == 0 {
				continue
			}
			found = append(found, Detection{
				Sample: eventSample, TimeSec: float64(eventSample) / float64(a.SampleRate),
				Channels: []int{ch.index}, Confidence: ev.confidence,
				Flags: flags, Features: ev.features,
			})
		}
	}

	found = mergeDetections(found, a.SampleRate)
	rejected = mergeSuppressed(rejected, a.SampleRate)
	for i := range found {
		found[i].Waveform = waveformFor(a, found[i].Sample, opts.WaveformBefore, opts.WaveformAfter)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Sample < found[j].Sample })
	sort.Slice(rejected, func(i, j int) bool { return rejected[i].Sample < rejected[j].Sample })
	out.Detections = found
	out.Suppressed = rejected
	out.SuppressedTotal = len(rejected)
	return out
}

func prepareChannel(a *wav.Audio, ch int) *channelData {
	x := make([]float64, a.Frames)
	for i := 0; i < a.Frames; i++ {
		x[i] = a.Samples[i*a.Channels+ch]
	}
	jump := absDiff(x)
	slope := secondDiff(x)
	lf := filter(x, newLowpass(float64(a.SampleRate), 180))
	lfStep := stepEdgeDiff(lf, a.SampleRate)
	return &channelData{
		index:      ch,
		x:          x,
		jump:       jump,
		slope:      slope,
		lowPass:    lf,
		lfStep:     lfStep,
		jumpProf:   profiles(jump, a.SampleRate),
		slopeProf:  profiles(slope, a.SampleRate),
		lfStepProf: profiles(lfStep, a.SampleRate),
	}
}

func profiles(v []float64, sampleRate int) []blockProfile {
	size := int(math.Round(float64(sampleRate) * blockSeconds))
	if size < 16 {
		size = 16
	}
	count := (len(v) + size - 1) / size
	out := make([]blockProfile, count)
	buf := make([]float64, 0, size)
	for k := 0; k < count; k++ {
		start := k * size
		end := start + size
		if end > len(v) {
			end = len(v)
		}
		buf = buf[:0]
		for _, x := range v[start:end] {
			buf = append(buf, x)
		}
		sort.Float64s(buf)
		out[k] = blockProfile{
			median: percentileSorted(buf, 0.5),
			p99:    percentileSorted(buf, 0.99),
		}
	}
	return out
}

func localLimit(prof []blockProfile, n, sampleRate int) (medianValue, p99 float64) {
	if len(prof) == 0 {
		return 0, 0
	}
	size := int(math.Round(float64(sampleRate) * blockSeconds))
	if size < 16 {
		size = 16
	}
	k := n / size
	const radius = 3
	var meds, p99s [2*radius + 1]float64
	count := 0
	for j := k - radius; j <= k+radius; j++ {
		if j < 0 || j >= len(prof) {
			continue
		}
		meds[count] = prof[j].median
		p99s[count] = prof[j].p99
		count++
	}
	insertionSort(meds[:count])
	insertionSort(p99s[:count])
	return percentileSorted(meds[:count], 0.5), percentileSorted(p99s[:count], 0.5)
}

func insertionSort(v []float64) {
	for i := 1; i < len(v); i++ {
		x := v[i]
		j := i
		for j > 0 && v[j-1] > x {
			v[j] = v[j-1]
			j--
		}
		v[j] = x
	}
}

func scanCandidates(ch *channelData, sampleRate int) []candidate {
	gap := int(math.Round(float64(sampleRate) * candidateGapSec))
	if gap < 2 {
		gap = 2
	}
	var out []candidate
	out = append(out, scanSeries(ch.jump, ch.jumpProf, sampleRate, gap, 2.4, 8, false)...)
	out = append(out, scanSeries(ch.slope, ch.slopeProf, sampleRate, gap, 2.8, 10, false)...)
	out = append(out, scanSeries(ch.lfStep, ch.lfStepProf, sampleRate, gap, 2.2, 6, true)...)
	out = mergeCandidates(out, sampleRate)
	margin := int(math.Round(float64(sampleRate) * 0.030))
	filtered := out[:0]
	for _, c := range out {
		if c.lowFreq && (c.sample < margin || c.sample >= len(ch.x)-margin) {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

func scanSeries(v []float64, prof []blockProfile, sampleRate, gap int,
	p99Factor, medianFactor float64, lowFreq bool) []candidate {
	var raw []candidate
	var group []int
	flush := func() {
		if len(group) == 0 {
			return
		}
		first := group[0]
		for _, n := range group {
			if v[n] > v[first] {
				first = n
			}
		}
		// Keep the leading edge for an impulse (two opposite deltas) but
		// allow a stronger sample one step later to win.
		for _, n := range group {
			if n <= group[0]+2 && v[n] >= 0.75*v[first] {
				first = n
				break
			}
		}
		raw = append(raw, candidate{sample: first, lowFreq: lowFreq})
		group = group[:0]
	}

	for n := 1; n < len(v); n++ {
		med, p99 := localLimit(prof, n, sampleRate)
		limit := math.Max(medianFactor*med, p99Factor*p99)
		limit = math.Max(limit, 1e-6)
		if v[n] >= limit {
			if len(group) > 0 && n-group[len(group)-1] > gap {
				flush()
			}
			group = append(group, n)
		}
	}
	flush()
	return raw
}

func mergeCandidates(in []candidate, sampleRate int) []candidate {
	if len(in) == 0 {
		return nil
	}
	sort.Slice(in, func(i, j int) bool { return in[i].sample < in[j].sample })
	tol := int(math.Round(float64(sampleRate) * mergeSeconds))
	if tol < 1 {
		tol = 1
	}
	lowFreqTol := int(math.Round(float64(sampleRate) * 0.004))
	out := []candidate{in[0]}
	for _, c := range in[1:] {
		last := &out[len(out)-1]
		limit := tol
		if c.lowFreq || last.lowFreq {
			limit = lowFreqTol
		}
		if c.sample-last.sample <= limit {
			last.lowFreq = last.lowFreq || c.lowFreq
			if c.sample < last.sample {
				last.sample = c.sample
			}
			continue
		}
		out = append(out, c)
	}
	return out
}
