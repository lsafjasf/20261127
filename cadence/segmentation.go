package cadence

import (
	"fmt"
	"math"
)

type frameRun struct {
	start int
	end   int
	state int
}

type rangeDecision struct {
	state int
	score windowScore
}

func basicRuns(labels []int) []frameRun {
	if len(labels) == 0 {
		return nil
	}
	out := []frameRun{{start: 0, state: labels[0]}}
	for i := 1; i < len(labels); i++ {
		if labels[i] != out[len(out)-1].state {
			out[len(out)-1].end = i
			out = append(out, frameRun{start: i, state: labels[i]})
		}
	}
	out[len(out)-1].end = len(labels)
	return out
}

func collapseUnknownTransitions(runs []frameRun, frames []Frame,
	sequences map[FieldOrder]fieldSequence, hint FieldOrderHint, opts Options) []frameRun {
	if len(runs) < 3 {
		return runs
	}
	grace := max(8, opts.WindowFrames*3/4)
	for i := 1; i+1 < len(runs); i++ {
		run := runs[i]
		if run.state != unknownState || run.end-run.start > grace {
			continue
		}
		left, right := runs[i-1], runs[i+1]
		if left.state == unknownState || right.state == unknownState {
			continue
		}
		if left.state == right.state {
			runs[i-1].end = right.end
			runs = append(runs[:i], runs[i+2:]...)
			i--
			continue
		}
		low := max(left.start+opts.MinSegmentFrames, run.start-2)
		high := min(right.end-opts.MinSegmentFrames, run.end+2)
		if high < low {
			continue
		}
		boundary := bestCadenceBoundary(left.state, right.state, left.start, right.end,
			low, high, frames, sequences, hint, opts)
		left.end = boundary
		right.start = boundary
		runs[i-1] = left
		runs[i+1] = right
		runs = append(runs[:i], runs[i+1:]...)
		i--
	}
	return runs
}

func mergeWeakRuns(runs []frameRun, frames []Frame,
	sequences map[FieldOrder]fieldSequence, hint FieldOrderHint, opts Options) []frameRun {
	if len(runs) < 2 {
		return runs
	}
	out := append([]frameRun(nil), runs...)
	for pass := 0; pass < 8; pass++ {
		changed := false
		for i := 0; i < len(out)-1; i++ {
			left, right := out[i], out[i+1]
			if left.state == unknownState || right.state == unknownState {
				continue
			}
			leftDecision := decideRange(left.start, left.end, sequences, hint)
			rightDecision := decideRange(right.start, right.end, sequences, hint)
			combined := decideRange(left.start, right.end, sequences, hint)
			if !shouldMergeWeakRuns(left, right, leftDecision, rightDecision, combined, opts) {
				continue
			}
			out[i] = frameRun{start: left.start, end: right.end, state: combined.state}
			out = append(out[:i+1], out[i+2:]...)
			changed = true
			break
		}
		if !changed {
			break
		}
	}
	return out
}

func recoverUnknownRuns(runs []frameRun, frames []Frame,
	sequences map[FieldOrder]fieldSequence, hint FieldOrderHint, opts Options) []frameRun {
	for i := 0; i < len(runs); i++ {
		run := runs[i]
		if run.state != unknownState || run.end-run.start < max(2*opts.MinSegmentFrames, opts.WindowFrames) {
			continue
		}
		bestStart, bestEnd := run.start, run.end
		best := rangeDecision{}
		consider := func(start, end int) {
			if end-start < opts.MinSegmentFrames {
				return
			}
			decision := decideRange(start, end, sequences, hint)
			if decision.score.score > best.score.score {
				bestStart, bestEnd, best = start, end, decision
			}
		}
		consider(run.start, run.end)
		for start := run.start; start <= min(run.end-opts.MinSegmentFrames, run.start+opts.WindowFrames); start++ {
			consider(start, run.end)
		}
		for end := run.end; end >= max(run.start+opts.MinSegmentFrames, run.end-opts.WindowFrames); end-- {
			consider(run.start, end)
		}
		if best.score.score < 0.45 {
			continue
		}
		var replacement []frameRun
		if bestStart > run.start {
			replacement = append(replacement, frameRun{start: run.start, end: bestStart, state: unknownState})
		}
		replacement = append(replacement, frameRun{start: bestStart, end: bestEnd, state: best.state})
		if bestEnd < run.end {
			replacement = append(replacement, frameRun{start: bestEnd, end: run.end, state: unknownState})
		}
		runs = append(runs[:i], append(replacement, runs[i+1:]...)...)
		i += len(replacement) - 1
	}
	return runs
}

func shouldMergeWeakRuns(left, right frameRun, leftScore, rightScore, combined rangeDecision,
	opts Options) bool {
	if combined.score.cadence == CadenceUnknown || combined.score.score < 0.55 {
		return false
	}
	leftWeak := left.end-left.start < opts.MinSegmentFrames*2 || leftScore.score.score < 0.56
	rightWeak := right.end-right.start < opts.MinSegmentFrames*2 || rightScore.score.score < 0.56
	if !leftWeak && !rightWeak {
		return false
	}
	if leftScore.score.cadence == rightScore.score.cadence &&
		combined.score.cadence == leftScore.score.cadence {
		return true
	}
	bestSide := math.Max(leftScore.score.score, rightScore.score.score)
	return combined.score.score >= bestSide-0.10 && combined.score.score >= 0.62
}

func decideRange(start, end int, sequences map[FieldOrder]fieldSequence,
	hint FieldOrderHint) rangeDecision {
	scores := make([]windowScore, len(cadenceStates))
	for i, st := range cadenceStates {
		seq := sequences[st.order].window(start*2, end*2)
		if st.cadence == Cadence24p32 {
			scores[i] = seq.score24p(hint)
		} else {
			scores[i] = seq.score30p(hint)
		}
	}
	applyOrderGate(scores, sequences[OrderTFF].window(start*2, end*2),
		sequences[OrderBFF].window(start*2, end*2), hint)
	best := chooseOrderWithTieBreak(scores, hint)
	return rangeDecision{state: best, score: scores[best]}
}

func detectCuts(frames []Frame, threshold float64) map[int]bool {
	cuts := make(map[int]bool)
	if len(frames) < 2 {
		return cuts
	}
	deltas := make([]float64, len(frames)-1)
	for i := 1; i < len(frames); i++ {
		deltas[i-1] = frameDistance(frames[i-1], frames[i])
	}
	base := median(deltas)
	adaptive := math.Max(threshold, base*3.2)
	for i, delta := range deltas {
		if delta >= threshold && delta >= adaptive {
			cuts[i+1] = true
		}
	}
	return cuts
}

func frameDistance(a, b Frame) float64 {
	da := combineDescriptors(makeDescriptor(a.Top), makeDescriptor(a.Bottom))
	db := combineDescriptors(makeDescriptor(b.Top), makeDescriptor(b.Bottom))
	return fieldDistance(da, db)
}

func combineDescriptors(a, b descriptor) descriptor {
	values := make([]float64, len(a.values))
	for i := range values {
		values[i] = (a.values[i] + b.values[i]) / 2
	}
	return descriptor{values: values, mean: (a.mean + b.mean) / 2}
}

func refineRuns(runs []frameRun, frames []Frame, sequences map[FieldOrder]fieldSequence,
	hint FieldOrderHint, opts Options, cuts map[int]bool) []frameRun {
	if len(runs) < 2 {
		return runs
	}
	out := append([]frameRun(nil), runs...)
	half := opts.WindowFrames / 2
	for i := 0; i < len(out)-1; i++ {
		left, right := &out[i], &out[i+1]
		if left.state == right.state {
			continue
		}
		original := right.start
		low := max(left.start+opts.MinSegmentFrames, original-half)
		high := min(right.end-opts.MinSegmentFrames, original+half)
		if high < low {
			continue
		}
		bestBoundary := bestCadenceBoundary(left.state, right.state,
			left.start, right.end, low, high, frames, sequences, hint, opts)
		// A hard cut near the transition is the strongest possible boundary.
		for cut := max(original-half, low); cut <= min(original+half, high); cut++ {
			if cuts[cut] && math.Abs(float64(cut-original)) <= 2 {
				bestBoundary = cut
				break
			}
		}
		left.end = bestBoundary
		right.start = bestBoundary
	}
	return out
}

func bestCadenceBoundary(leftState, rightState, leftStart, rightEnd,
	low, high int, frames []Frame, sequences map[FieldOrder]fieldSequence,
	hint FieldOrderHint, opts Options) int {
	side := max(8, min(16, opts.WindowFrames))
	original := (low + high) / 2
	bestBoundary := original
	bestValue := math.Inf(-1)
	for boundary := low; boundary <= high; boundary++ {
		leftScore := localStateScore(leftState, sequences, frames,
			max(leftStart, boundary-side), boundary, hint)
		rightScore := localStateScore(rightState, sequences, frames,
			boundary, min(rightEnd, boundary+side), hint)
		value := leftScore + rightScore
		value -= 0.001 * math.Abs(float64(boundary-original))
		if value > bestValue {
			bestValue = value
			bestBoundary = boundary
		}
	}
	return bestBoundary
}

func localStateScore(label int, sequences map[FieldOrder]fieldSequence, frames []Frame,
	start, end int, hint FieldOrderHint) float64 {
	if label < 0 || label >= len(cadenceStates) || end <= start {
		return 0
	}
	st := cadenceStates[label]
	seq := sequences[st.order].window(start*2, end*2)
	if st.cadence == Cadence24p32 {
		return seq.score24p(hint).score
	}
	return seq.score30p(hint).score
}

func buildSegment(index, start, end int, media Media,
	sequences map[FieldOrder]fieldSequence, hint FieldOrderHint) Segment {
	seg := Segment{
		Index:      index,
		StartFrame: start,
		EndFrame:   end,
		StartField: start * 2,
		EndField:   end * 2,
		StartSec:   round3(float64(start) / media.FrameRate),
		EndSec:     round3(float64(end) / media.FrameRate),
	}
	scores := scoreRange(start, end, sequences, hint)
	best := chooseOrderWithTieBreak(scores, hint)
	bestScore := scores[best]
	otherCadence := bestOfOtherCadence(scores, bestScore.cadence)
	cadenceMargin := bestScore.score - scores[otherCadence].score
	fields := (end - start) * 2
	unknown := segmentNeedsUnknown(bestScore, cadenceMargin, fields)
	if unknown {
		seg.Cadence = CadenceUnknown
		seg.FieldOrder = OrderUnknown
		seg.Confidence = round3(clamp(0.62-bestScore.score, 0.05, 0.55))
		seg.Recommended = "isolate_and_review"
	} else {
		seg.Cadence = bestScore.cadence
		seg.FieldOrder = bestScore.order
		seg.Confidence = round3(segmentConfidence(bestScore, cadenceMargin, fields))
		seg.Recommended = recommendedAction(bestScore.cadence)
	}
	seg.Evidence = segmentEvidence(bestScore, scores, best, hint, unknown, fields)
	return seg
}

func scoreRange(start, end int, sequences map[FieldOrder]fieldSequence,
	hint FieldOrderHint) []windowScore {
	scores := make([]windowScore, len(cadenceStates))
	for i, st := range cadenceStates {
		seq := sequences[st.order].window(start*2, end*2)
		if st.cadence == Cadence24p32 {
			scores[i] = seq.score24p(hint)
		} else {
			scores[i] = seq.score30p(hint)
		}
	}
	applyOrderGate(scores, sequences[OrderTFF].window(start*2, end*2),
		sequences[OrderBFF].window(start*2, end*2), hint)
	return scores
}

func applyOrderGate(scores []windowScore, tff, bff fieldSequence, hint FieldOrderHint) {
	if hint == HintTFF || hint == HintBFF {
		return
	}
	ocT := orderContinuity(tff)
	ocB := orderContinuity(bff)
	penalize := func(order FieldOrder, factor float64) {
		for i := range scores {
			if scores[i].order == order {
				scores[i].score *= factor
			}
		}
	}
	switch {
	case ocT >= 0.62 && ocB <= 0.30 && ocT-ocB >= 0.40:
		penalize(OrderBFF, 0.20)
	case ocB >= 0.62 && ocT <= 0.30 && ocB-ocT >= 0.40:
		penalize(OrderTFF, 0.20)
	}
}

func chooseOrderWithTieBreak(scores []windowScore, hint FieldOrderHint) int {
	best := 0
	for i := 1; i < len(scores); i++ {
		if scores[i].score > scores[best].score {
			best = i
		}
	}
	preferred := -1
	for i := range scores {
		if i == best || scores[i].cadence != scores[best].cadence {
			continue
		}
		if hint == HintTFF && scores[i].order == OrderTFF {
			preferred = i
		}
		if hint == HintBFF && scores[i].order == OrderBFF {
			preferred = i
		}
		if scores[best].cadence == Cadence30p22 && preferred < 0 && scores[i].order == OrderTFF {
			preferred = i
		}
	}
	if preferred >= 0 && scores[best].score-scores[preferred].score <= 0.035 {
		return preferred
	}
	return best
}

func bestOfOtherCadence(scores []windowScore, cadence Cadence) int {
	best := -1
	for i := range scores {
		if scores[i].cadence == cadence {
			continue
		}
		if best < 0 || scores[i].score > scores[best].score {
			best = i
		}
	}
	if best < 0 {
		return 0
	}
	return best
}

func segmentNeedsUnknown(best windowScore, cadenceMargin float64, fields int) bool {
	if best.score < 0.36 {
		return true
	}
	if best.staticRatio > 0.72 && best.medianMotion < defaultStillFloor*2 {
		return true
	}
	if fields < 10 && best.score < 0.78 {
		return true
	}
	if best.score < 0.62 && cadenceMargin < 0.05 {
		return true
	}
	return false
}

func segmentConfidence(best windowScore, cadenceMargin float64, fields int) float64 {
	margin := clamp(cadenceMargin, 0, 1)
	quality := best.repeatConsistency
	if best.cadence == Cadence30p22 {
		quality = best.periodFit
	}
	conf := 0.52*best.score + 0.28*margin + 0.20*quality
	if best.cadence == Cadence24p32 && fields < 20 {
		conf *= 0.85
	}
	return clamp(conf, 0.05, 0.99)
}

func recommendedAction(c Cadence) string {
	switch c {
	case Cadence24p32:
		return "inverse_telecine_24p"
	case Cadence30p22:
		return "treat_as_30p_field_pairs"
	default:
		return "isolate_and_review"
	}
}

func segmentEvidence(best windowScore, scores []windowScore, bestIndex int,
	hint FieldOrderHint, unknown bool, fields int) Evidence {
	orderScore := fieldOrderConfidence(best, scores, bestIndex, hint)
	ev := Evidence{
		RepeatObserved:     best.repeatObserved,
		RepeatExpected:     best.repeatExpected,
		RepeatConsistency:  round3(best.repeatConsistency),
		RepeatPhase:        best.repeatPhase,
		PeriodFit:          round3(best.periodFit),
		MixedFieldPairs:    best.mixedPairs,
		CoherentFieldPairs: best.coherentPairs,
		CoherentPairRatio:  round3(pairRatio(best)),
		WithinContrast:     round3(best.withinContrast),
		MotionContinuity:   round3(best.motionContinuity),
		FieldOrderScore:    round3(orderScore),
		WithinPairMotion:   round6(best.withinMotion),
		AcrossPairMotion:   round6(best.acrossMotion),
		MedianFieldMotion:  round6(best.medianMotion),
		SceneCuts:          best.cuts,
		StaticRatio:        round3(best.staticRatio),
		AnalyzedFields:     fields,
	}
	ev.PeriodFields = 10
	if best.cadence == Cadence30p22 {
		ev.PeriodFields = 2
	}
	ev.FieldOrderSource = fieldOrderSource(best, scores, bestIndex, hint)
	if unknown {
		ev.PeriodFields = 0
		ev.Basis = append(ev.Basis,
			fmt.Sprintf("cadence evidence is insufficient (24p fit %.2f, 30p fit %.2f)",
				scores[0].score, scores[2].score),
			"only this segment is marked unknown")
	} else if best.cadence == Cadence24p32 {
		ev.Basis = append(ev.Basis,
			fmt.Sprintf("%d/%d expected mixed field-pair slots match the 3:2 pattern at phase %d",
				best.repeatObserved, best.repeatExpected, best.repeatPhase),
			fmt.Sprintf("repeating 10-field cycle fit %.2f", best.repeatConsistency),
			fmt.Sprintf("field order %s supported by %s", best.order, ev.FieldOrderSource))
		if best.staticRatio > 0.25 {
			ev.Basis = append(ev.Basis, "part of the segment has little motion; confidence is reduced")
		}
	} else {
		ev.Basis = append(ev.Basis,
			fmt.Sprintf("two-field cadence: intra-pair %.5f, inter-pair %.5f",
				best.withinMotion, best.acrossMotion),
			fmt.Sprintf("coherent-pair ratio %.2f; no stable 3:2 repeat cycle", ev.CoherentPairRatio),
			fmt.Sprintf("field order %s supported by %s", best.order, ev.FieldOrderSource))
	}
	if best.cuts > 0 {
		ev.Basis = append(ev.Basis,
			fmt.Sprintf("%d scene cut(s) kept inside the evidence window", best.cuts))
	}
	if len(ev.Basis) == 0 || (best.score < 0.55 && !unknown) {
		ev.Basis = append(ev.Basis, "decision uses local evidence only; no whole-file fallback")
	}
	return ev
}

func pairRatio(score windowScore) float64 {
	if score.validPairs == 0 {
		return 0
	}
	return float64(score.coherentPairs) / float64(score.validPairs)
}

func fieldOrderConfidence(best windowScore, scores []windowScore, bestIndex int,
	hint FieldOrderHint) float64 {
	otherBest := math.Inf(-1)
	for i := range scores {
		if i == bestIndex || scores[i].cadence != best.cadence {
			continue
		}
		if scores[i].score > otherBest {
			otherBest = scores[i].score
		}
	}
	margin := 0.0
	if !math.IsInf(otherBest, -1) {
		margin = best.score - otherBest
	}
	score := clamp(0.50+2.5*math.Max(0, margin), 0, 1)
	if (hint == HintTFF && best.order == OrderTFF) || (hint == HintBFF && best.order == OrderBFF) {
		score = math.Max(score, 0.72)
	}
	return score
}

func fieldOrderSource(best windowScore, scores []windowScore, bestIndex int, hint FieldOrderHint) string {
	otherOrder := -1
	for i := range scores {
		if cadenceStates[i].cadence == best.cadence && i != bestIndex {
			otherOrder = i
			break
		}
	}
	margin := 0.0
	if otherOrder >= 0 {
		margin = best.score - scores[otherOrder].score
	}
	hinted := (hint == HintTFF && best.order == OrderTFF) ||
		(hint == HintBFF && best.order == OrderBFF)
	switch {
	case margin > 0.06:
		return "motion continuity"
	case hinted:
		return "container metadata and content tie"
	case best.cadence == Cadence30p22 && best.order == OrderTFF:
		return "unobservable 2:2 order; defaulted to TFF"
	default:
		return "weak content tie"
	}
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
