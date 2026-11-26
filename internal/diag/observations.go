package diag

import (
	"fmt"
	"math"
	"sort"

	"seekdiag/internal/mp4"
)

func expectedAudioPTS(t *mp4.Track, o Observation) (float64, bool) {
	if t == nil {
		return 0, false
	}
	if o.AudioPCMOffset != nil {
		rate := t.SampleRate
		if rate == 0 {
			rate = t.Timescale
		}
		if rate == 0 {
			return 0, false
		}
		return float64(*o.AudioPCMOffset) * 1000 / float64(rate), true
	}
	var mediaTick int64
	found := false
	switch {
	case o.AudioSampleIndex != nil:
		index := int(*o.AudioSampleIndex) - 1
		if index >= 0 && index < len(t.Samples) {
			mediaTick, found = t.Samples[index].PTS(), true
		}
	case o.AudioByteOffset != nil:
		for _, s := range t.Samples {
			if *o.AudioByteOffset >= int64(s.FileOffset) && *o.AudioByteOffset < int64(s.FileOffset)+int64(s.Size) {
				mediaTick, found = s.PTS(), true
				break
			}
		}
	}
	if !found {
		return 0, false
	}
	movieTick, ok := t.MediaToMovie(mediaTick)
	if !ok {
		return 0, false
	}
	return ticksToMS(movieTick, t.MovieTimescale), true
}

type probePair struct {
	x, y float64
}

func buildProbe(t *mp4.Track, o Observation) ProbeView {
	p := ProbeView{Observation: o}
	expected, hasExpected := expectedAudioPTS(t, o)
	if hasExpected {
		p.ExpectedAudioPTSMS = ptr(expected)
	}
	if o.AudioPTSMS != nil && hasExpected {
		p.ContainerErrorMS = ptr(*o.AudioPTSMS - expected)
	}
	if o.DecodedAudioPTSMS != nil {
		ref, ok := 0.0, false
		if o.AudioPTSMS != nil {
			ref, ok = *o.AudioPTSMS, true
		} else if hasExpected {
			ref, ok = expected, true
		}
		if ok {
			p.DecoderErrorMS = ptr(*o.DecodedAudioPTSMS - ref)
		}
	}
	if o.PlaybackClockMS != nil {
		ref, ok := 0.0, false
		if o.DecodedAudioPTSMS != nil {
			ref, ok = *o.DecodedAudioPTSMS, true
		} else if o.AudioPTSMS != nil {
			ref, ok = *o.AudioPTSMS, true
		} else if hasExpected {
			ref, ok = expected, true
		}
		if ok {
			p.ClockErrorMS = ptr(*o.PlaybackClockMS - ref)
		}
	}
	return p
}

func driftOf(layer string, points []probePair) (DriftView, bool) {
	if len(points) < 2 {
		return DriftView{}, false
	}
	sort.Slice(points, func(i, j int) bool { return points[i].x < points[j].x })
	start, end := points[0], points[len(points)-1]
	spanMS := end.x - start.x
	if spanMS <= 0 {
		return DriftView{}, false
	}
	var sx, sy, sxx, sxy float64
	for _, p := range points {
		sx += p.x
		sy += p.y
		sxx += p.x * p.x
		sxy += p.x * p.y
	}
	n := float64(len(points))
	denom := n*sxx - sx*sx
	slope := 0.0
	if math.Abs(denom) > 1e-9 {
		slope = (n*sxy - sx*sy) / denom * 1000
	}
	return DriftView{
		Layer: layer, StartMS: start.y, EndMS: end.y,
		DeltaMS: end.y - start.y, SlopeMSps: slope, SpanMS: spanMS, Samples: len(points),
	}, true
}

func analyzeObservations(tracks []*mp4.Track, set *ObservationSet, opts Options) (*ObservationReport, []Finding, string) {
	var audio *mp4.Track
	for _, t := range tracks {
		if t.Kind() == "audio" {
			audio = t
			break
		}
	}
	report := &ObservationReport{}
	for _, o := range set.Before {
		report.Before = append(report.Before, buildProbe(audio, o))
	}
	for _, o := range set.After {
		report.After = append(report.After, buildProbe(audio, o))
	}
	metrics := []struct {
		layer string
		get   func(ProbeView) *float64
	}{
		{VerdictContainer, func(p ProbeView) *float64 { return p.ContainerErrorMS }},
		{VerdictDecoder, func(p ProbeView) *float64 { return p.DecoderErrorMS }},
		{VerdictClock, func(p ProbeView) *float64 { return p.ClockErrorMS }},
	}
	var findings []Finding
	var drifts []DriftView
	for _, metric := range metrics {
		var pairs []probePair
		for _, p := range report.After {
			if v := metric.get(p); v != nil {
				pairs = append(pairs, probePair{x: p.WallMS, y: *v})
			}
		}
		d, ok := driftOf(metric.layer, pairs)
		if !ok {
			continue
		}
		drifts = append(drifts, d)
		if math.Abs(d.DeltaMS) >= opts.MinDriftMS && math.Abs(d.SlopeMSps) >= opts.MinDriftMSps {
			findings = append(findings, Finding{
				Layer: metric.layer, Code: "post_seek_drift", Severity: "error", ValueMS: d.DeltaMS,
				Message: fmt.Sprintf("%s evidence drifts %.2f ms over %.2f s (%.3f ms/s)", metric.layer, d.DeltaMS, d.SpanMS/1000, d.SlopeMSps),
			})
		} else if math.Abs(d.EndMS) >= opts.BoundaryTolMS {
			findings = append(findings, Finding{
				Layer: metric.layer, Code: "post_seek_offset", Severity: "warning", ValueMS: d.EndMS,
				Message: fmt.Sprintf("%s evidence ends at a non-zero offset of %.2f ms", metric.layer, d.EndMS),
			})
		}
	}
	report.Drift = drifts
	return report, findings, pickObservationVerdict(findings, len(report.After))
}

func pickObservationVerdict(findings []Finding, afterCount int) string {
	if afterCount < 2 {
		return VerdictInconclusive
	}
	for _, layer := range []string{VerdictContainer, VerdictDecoder, VerdictClock} {
		for _, f := range findings {
			if f.Layer == layer && f.Severity == "error" {
				return layer
			}
		}
	}
	return VerdictOK
}

func ptr(v float64) *float64 { return &v }
