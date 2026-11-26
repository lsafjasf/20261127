package click

import (
	"sort"

	"clickdetect/internal/wav"
)

func mergeDetections(in []Detection, sampleRate int) []Detection {
	if len(in) == 0 {
		return []Detection{}
	}
	tol := int(float64(sampleRate) * mergeSeconds)
	if tol < 1 {
		tol = 1
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Sample < in[j].Sample })
	out := []Detection{in[0]}
	for _, d := range in[1:] {
		last := &out[len(out)-1]
		if d.Sample-last.Sample > tol {
			out = append(out, d)
			continue
		}
		last.Channels = unionInts(last.Channels, d.Channels)
		if d.Confidence > last.Confidence {
			last.Sample = d.Sample
			last.TimeSec = d.TimeSec
			last.Confidence = d.Confidence
			last.Flags = d.Flags
			last.Features = d.Features
		}
	}
	return out
}

func mergeSuppressed(in []Suppressed, sampleRate int) []Suppressed {
	if len(in) == 0 {
		return []Suppressed{}
	}
	tol := int(float64(sampleRate) * mergeSeconds)
	if tol < 1 {
		tol = 1
	}
	sort.Slice(in, func(i, j int) bool { return in[i].Sample < in[j].Sample })
	out := []Suppressed{in[0]}
	for _, s := range in[1:] {
		last := &out[len(out)-1]
		if s.Sample-last.Sample > tol {
			out = append(out, s)
			continue
		}
		last.Channels = unionInts(last.Channels, s.Channels)
	}
	return out
}

func unionInts(a, b []int) []int {
	seen := make(map[int]bool, len(a)+len(b))
	out := make([]int, 0, len(a)+len(b))
	for _, v := range append(append([]int(nil), a...), b...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func waveformFor(a *wav.Audio, sample, before, after int) Waveform {
	first := sample - before
	if first < 0 {
		first = 0
	}
	last := sample + after
	if last >= a.Frames {
		last = a.Frames - 1
	}
	if last < first {
		return Waveform{FirstSample: first, ClickSample: sample, Samples: [][]float64{}}
	}
	n := last - first + 1
	out := make([][]float64, a.Channels)
	for ch := 0; ch < a.Channels; ch++ {
		out[ch] = make([]float64, n)
		for i := 0; i < n; i++ {
			out[ch][i] = a.Samples[(first+i)*a.Channels+ch]
		}
	}
	return Waveform{FirstSample: first, ClickSample: sample, Samples: out}
}
