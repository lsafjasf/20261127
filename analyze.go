package avsync

import (
	"fmt"
	"math"
	"sort"
)

// Analyze pairs pulse and visual events independently inside every segment,
// then fits e = offset + drift*elapsed for each segment. No cross-segment
// offset or drift is shared.
func Analyze(in Input) (Result, error) {
	segs, pulses, events, opts, err := in.normalize()
	if err != nil {
		return Result{}, err
	}
	works := make([]segmentWork, len(segs))
	for i, s := range segs {
		works[i] = segmentWork{spec: s, startMS: s.Start, endMS: s.End}
	}
	assignPoints(works, pulses, events)

	for i := range works {
		fitSegment(&works[i], opts)
	}

	res := Result{
		TimeUnit:   "ms",
		OffsetUnit: "ms",
		Segments:   make([]SegmentResult, 0, len(works)),
	}
	allPairs := make([]pair, 0)
	beforeN, afterN := 0, 0
	for i := range works {
		w := &works[i]
		sr := segmentResult(w, opts)
		res.Segments = append(res.Segments, sr)
		res.MatchedPairs += len(w.pairs)
		res.Before = combineStats(res.Before, rawStats(w.pairs), beforeN, len(w.pairs))
		res.After = combineStats(res.After, correctedStats(w.pairs, w.fit, w.startMS), afterN, len(w.pairs))
		beforeN += len(w.pairs)
		afterN += len(w.pairs)
		allPairs = append(allPairs, w.pairs...)
	}
	res.UnmatchedPulse = len(pulses) - res.MatchedPairs
	res.UnmatchedEvent = len(events) - res.MatchedPairs
	if res.UnmatchedPulse < 0 {
		res.UnmatchedPulse = 0
	}
	if res.UnmatchedEvent < 0 {
		res.UnmatchedEvent = 0
	}

	markCutDiscontinuities(res.Segments, works, opts.OffsetToleranceMS)
	res.FirstAnomaly = firstAnomaly(res.Segments)
	res.GlobalAfter = globalStats(allPairs)
	return res, nil
}

func assignPoints(works []segmentWork, pulses, events []float64) {
	for _, p := range pulses {
		if i := segmentIndex(works, p); i >= 0 {
			works[i].pulses = append(works[i].pulses, p)
		}
	}
	for _, e := range events {
		if i := segmentIndex(works, e); i >= 0 {
			works[i].events = append(works[i].events, e)
		}
	}
}

func segmentIndex(works []segmentWork, t float64) int {
	i := sort.Search(len(works), func(i int) bool { return works[i].startMS > t }) - 1
	if i < 0 || t >= works[i].endMS {
		return -1
	}
	return i
}

func fitSegment(w *segmentWork, opts Options) {
	if len(w.pulses) < 2 || len(w.events) < 2 {
		return
	}
	pred := lineFit{offset: initialOffset(w.pulses, w.events, opts.MatchWindowMS)}
	var previous []pair
	for iter := 0; iter < 6; iter++ {
		pairs := matchPoints(w.pulses, w.events, pred, w.startMS, opts.MatchWindowMS)
		if len(pairs) < 2 {
			return
		}
		fit, err := fitLine(pairs, w.startMS, opts.MinDriftSpanMS)
		if err != nil {
			return
		}
		w.pairs, w.fit, pred = pairs, fit, fit
		if samePairs(previous, pairs) {
			return
		}
		previous = pairs
	}
}

func samePairs(a, b []pair) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].pulseMS != b[i].pulseMS || a[i].eventMS != b[i].eventMS {
			return false
		}
	}
	return true
}

func segmentResult(w *segmentWork, opts Options) SegmentResult {
	before := rawStats(w.pairs)
	after := correctedStats(w.pairs, w.fit, w.startMS)
	duration := w.endMS - w.startMS
	offsetEnd := w.fit.offset + w.fit.slope*duration
	span := 0.0
	if len(w.pairs) > 1 {
		span = w.pairs[len(w.pairs)-1].pulseMS - w.pairs[0].pulseMS
	}
	sr := SegmentResult{
		ID:            w.spec.ID,
		Source:        w.spec.Source,
		StartMS:       w.startMS,
		EndMS:         w.endMS,
		PairCount:     len(w.pairs),
		OffsetMS:      clean(w.fit.offset),
		OffsetEndMS:   clean(offsetEnd),
		DriftPPM:      clean(w.fit.slope * 1e6),
		DriftMSPerS:   clean(w.fit.slope * 1000),
		DriftGain:     1 + clean(w.fit.slope),
		RMSBeforeMS:   clean(before.RMSMS),
		MaxBeforeMS:   clean(before.MaxMS),
		RMSAfterMS:    clean(after.RMSMS),
		MaxAfterMS:    clean(after.MaxMS),
		EnoughPairs:   len(w.pairs) >= opts.MinPairs,
		DriftReliable: len(w.pairs) >= 3 && span >= opts.MinDriftSpanMS,
	}
	if !sr.EnoughPairs && (len(w.pulses) > 0 || len(w.events) > 0) {
		sr.Anomalous = true
		sr.AnomalyWhy = append(sr.AnomalyWhy, "insufficient_pairs")
	}
	if math.Abs(w.fit.offset) > opts.OffsetToleranceMS ||
		math.Abs(offsetEnd) > opts.OffsetToleranceMS {
		sr.Anomalous = true
		sr.AnomalyWhy = append(sr.AnomalyWhy, "offset_exceeds_tolerance")
	}
	if sr.DriftReliable && math.Abs(sr.DriftPPM) > opts.DriftTolerancePPM {
		sr.Anomalous = true
		sr.AnomalyWhy = append(sr.AnomalyWhy, "drift_exceeds_tolerance")
	}
	if after.MaxMS > opts.OffsetToleranceMS {
		sr.Anomalous = true
		sr.AnomalyWhy = append(sr.AnomalyWhy, "unmodeled_residual")
	}
	if t := firstBadPair(w.pairs, opts.OffsetToleranceMS); t != nil {
		sr.FirstBadMS = t
	}
	return sr
}

func markCutDiscontinuities(results []SegmentResult, works []segmentWork, toleranceMS float64) {
	for i := 1; i < len(results); i++ {
		if results[i-1].PairCount == 0 || results[i].PairCount == 0 {
			continue
		}
		distance := works[i].startMS - works[i-1].startMS
		prevAtCut := works[i-1].fit.offset + works[i-1].fit.slope*distance
		jump := works[i].fit.offset - prevAtCut
		if math.Abs(jump) > toleranceMS {
			results[i].Anomalous = true
			results[i].AnomalyWhy = append(results[i].AnomalyWhy, "cut_discontinuity")
		}
	}
}

func firstAnomaly(results []SegmentResult) *Anomaly {
	for i := range results {
		sr := &results[i]
		if !sr.Anomalous {
			continue
		}
		return &Anomaly{
			SegmentID:      sr.ID,
			Source:         sr.Source,
			SegmentStartMS: sr.StartMS,
			FirstBadMS:     sr.FirstBadMS,
			Reasons:        append([]string(nil), sr.AnomalyWhy...),
		}
	}
	return nil
}

func firstBadPair(ps []pair, toleranceMS float64) *float64 {
	for _, p := range ps {
		if math.Abs(p.eventMS-p.pulseMS) > toleranceMS {
			t := p.pulseMS
			return &t
		}
	}
	return nil
}

func rawStats(ps []pair) ErrorStats {
	var sum, max float64
	for _, p := range ps {
		e := p.eventMS - p.pulseMS
		sum += e * e
		max = math.Max(max, math.Abs(e))
	}
	return ErrorStats{RMSMS: clean(rms(sum, len(ps))), MaxMS: clean(max)}
}

func correctedStats(ps []pair, fit lineFit, anchorMS float64) ErrorStats {
	var sum, max float64
	for _, p := range ps {
		e := p.eventMS - p.pulseMS - fit.predict(p.pulseMS-anchorMS)
		sum += e * e
		max = math.Max(max, math.Abs(e))
	}
	return ErrorStats{RMSMS: clean(rms(sum, len(ps))), MaxMS: clean(max)}
}

func globalStats(ps []pair) ErrorStats {
	if len(ps) < 2 {
		return ErrorStats{}
	}
	fit, err := fitLine(ps, ps[0].pulseMS, 0)
	if err != nil {
		return ErrorStats{}
	}
	return correctedStats(ps, fit, ps[0].pulseMS)
}

func combineStats(a, b ErrorStats, an, bn int) ErrorStats {
	if an == 0 {
		return b
	}
	if bn == 0 {
		return a
	}
	total := an + bn
	return ErrorStats{
		RMSMS: math.Sqrt((a.RMSMS*a.RMSMS*float64(an) +
			b.RMSMS*b.RMSMS*float64(bn)) / float64(total)),
		MaxMS: math.Max(a.MaxMS, b.MaxMS),
	}
}

func rms(sumSquares float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return math.Sqrt(sumSquares / float64(n))
}

func clean(v float64) float64 {
	if math.Abs(v) < 5e-12 {
		return 0
	}
	return math.Round(v*1e9) / 1e9
}

func (r Result) String() string {
	return fmt.Sprintf("segments=%d pairs=%d first_anomaly=%v after_rms=%.3fms",
		len(r.Segments), r.MatchedPairs, r.FirstAnomaly, r.After.RMSMS)
}
