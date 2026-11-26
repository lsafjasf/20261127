package cadence

import "math"

type fieldObs struct {
	frame  int
	parity Parity
	desc   descriptor
}

type fieldSequence struct {
	order  FieldOrder
	fields []fieldObs
	gaps   []float64
}

func makeSequences(frames []Frame) (tff, bff fieldSequence) {
	tff = fieldSequence{
		order:  OrderTFF,
		fields: make([]fieldObs, 0, len(frames)*2),
		gaps:   make([]float64, 0, len(frames)*2-1),
	}
	bff = fieldSequence{
		order:  OrderBFF,
		fields: make([]fieldObs, 0, len(frames)*2),
		gaps:   make([]float64, 0, len(frames)*2-1),
	}
	for i, frame := range frames {
		top := fieldObs{frame: i, parity: Top, desc: makeDescriptor(frame.Top)}
		bottom := fieldObs{frame: i, parity: Bottom, desc: makeDescriptor(frame.Bottom)}
		tff.fields = append(tff.fields, top, bottom)
		bff.fields = append(bff.fields, bottom, top)
	}
	tff.gaps = adjacentGaps(tff.fields)
	bff.gaps = adjacentGaps(bff.fields)
	return tff, bff
}

func adjacentGaps(fields []fieldObs) []float64 {
	out := make([]float64, 0, len(fields)-1)
	for i := 1; i < len(fields); i++ {
		out = append(out, fieldDistance(fields[i-1].desc, fields[i].desc))
	}
	return out
}

func (s fieldSequence) window(startField, endField int) fieldSequence {
	startField = max(0, startField)
	endField = min(len(s.fields), endField)
	if endField <= startField {
		return fieldSequence{order: s.order}
	}
	fields := s.fields[startField:endField]
	gaps := adjacentGaps(fields)
	return fieldSequence{order: s.order, fields: fields, gaps: gaps}
}

func (s fieldSequence) orderHintPenalty(hint FieldOrderHint) float64 {
	switch hint {
	case HintTFF:
		if s.order != OrderTFF {
			return 0.08
		}
	case HintBFF:
		if s.order != OrderBFF {
			return 0.08
		}
	}
	return 0
}

type pairMotion struct {
	within    float64
	across    float64
	hasAcross bool
}

func (s fieldSequence) pairMotions() []pairMotion {
	out := make([]pairMotion, 0, len(s.fields)/2)
	for i := 0; i+1 < len(s.fields); i += 2 {
		item := pairMotion{
			within: fieldDistance(s.fields[i].desc, s.fields[i+1].desc),
		}
		if i+2 < len(s.fields) {
			item.across = fieldDistance(s.fields[i+1].desc, s.fields[i+2].desc)
			item.hasAcross = true
		}
		out = append(out, item)
	}
	return out
}

type windowScore struct {
	cadence           Cadence
	order             FieldOrder
	score             float64
	repeatObserved    int
	repeatExpected    int
	repeatConsistency float64
	repeatPhase       int
	periodFit         float64
	mixedPairs        int
	coherentPairs     int
	validPairs        int
	withinContrast    float64
	motionContinuity  float64
	fieldOrderScore   float64
	withinMotion      float64
	acrossMotion      float64
	medianMotion      float64
	cuts              int
	staticRatio       float64
	fields            int
	basis             []string
}

func (s fieldSequence) score24p(hint FieldOrderHint) windowScore {
	out := windowScore{
		cadence: Cadence24p32,
		order:   s.order,
		fields:  len(s.fields),
	}
	pairs := s.pairMotions()
	if len(pairs) < 5 {
		return out
	}
	out.withinMotion = medianPair(pairs, true)
	out.acrossMotion = medianPair(pairs, false)
	out.medianMotion = out.acrossMotion
	out.staticRatio = staticPairRatio(pairs, defaultStillFloor)
	out.cuts = countPairCuts(pairs, defaultCutThreshold)
	classes, mixed, coherent, valid := classifyPairs(pairs, defaultStillFloor)
	out.mixedPairs = mixed
	out.coherentPairs = coherent
	out.validPairs = valid
	out.withinContrast = pairWithinContrast(pairs)
	out.repeatConsistency, out.repeatPhase, out.repeatExpected, out.repeatObserved =
		scoreFilmPattern(classes, mixed, valid)
	out.periodFit = out.repeatConsistency
	out.motionContinuity = pairSmoothness(pairs)
	out.fieldOrderScore = orderContinuity(s)

	contrastFit := clamp((out.withinContrast-1.0)/2.0, 0, 1)
	score := 0.35*out.repeatConsistency + 0.40*contrastFit +
		0.15*out.motionContinuity + 0.10*out.fieldOrderScore
	score -= s.orderHintPenalty(hint)
	out.score = clamp(score, 0, 1)
	if out.repeatObserved > 0 {
		out.basis = append(out.basis,
			"three coherent and two mixed field pairs fit a five-frame cycle",
			"repeated pair positions stay stable across the window")
	}
	return out
}

func (s fieldSequence) score30p(hint FieldOrderHint) windowScore {
	out := windowScore{
		cadence: Cadence30p22,
		order:   s.order,
		fields:  len(s.fields),
	}
	pairs := s.pairMotions()
	if len(pairs) < 3 {
		return out
	}
	var within, across []float64
	for _, pair := range pairs {
		within = append(within, pair.within)
		if pair.hasAcross {
			across = append(across, pair.across)
		}
	}
	out.withinMotion = median(within)
	out.acrossMotion = median(across)
	out.medianMotion = out.acrossMotion
	out.staticRatio = staticPairRatio(pairs, defaultStillFloor)
	out.cuts = countPairCuts(pairs, defaultCutThreshold)
	separation := 0.0
	den := out.withinMotion + out.acrossMotion
	if den > 1e-9 {
		separation = clamp((out.acrossMotion-out.withinMotion)/den, 0, 1)
	}
	_, mixed, coherent, valid := classifyPairs(pairs, defaultStillFloor)
	out.mixedPairs = mixed
	out.coherentPairs = coherent
	out.validPairs = valid
	out.withinContrast = pairWithinContrast(pairs)
	pairRatio := 0.0
	if valid > 0 {
		pairRatio = float64(coherent) / float64(valid)
	}
	pairFit := clamp((pairRatio-0.78)/0.22, 0, 1)
	out.periodFit = pairFit
	out.motionContinuity = crossMotionSmoothness(across)
	out.fieldOrderScore = orderContinuity(s)

	consistency := clamp(1-out.withinContrast/2, 0, 1)
	score := 0.38*pairFit + 0.25*separation + 0.22*consistency + 0.15*out.fieldOrderScore
	score -= s.orderHintPenalty(hint)
	out.score = clamp(score, 0, 1)
	if separation > 0.35 && pairFit > 0.55 {
		out.basis = append(out.basis,
			"field pairs have coherent motion at the two-field cadence",
			"inter-pair motion exceeds intra-pair motion")
	}
	return out
}

const (
	defaultStillFloor   = 0.0005
	defaultCutThreshold = 0.24
)

const (
	pairNeutral = iota
	pairCoherent
	pairMixed
)

func classifyPairs(pairs []pairMotion, stillFloor float64) (classes []int, mixed, coherent, valid int) {
	classes = make([]int, len(pairs))
	var refs []float64
	for _, pair := range pairs {
		if pair.hasAcross && pair.across >= stillFloor && pair.across < defaultCutThreshold {
			refs = append(refs, pair.across)
		}
	}
	if len(refs) == 0 {
		return classes, 0, 0, 0
	}
	threshold := 0.55 * median(refs)
	for i, pair := range pairs {
		if !pair.hasAcross || pair.across < stillFloor || pair.across >= defaultCutThreshold {
			continue
		}
		valid++
		if pair.within > threshold {
			mixed++
			classes[i] = pairMixed
		} else {
			coherent++
			classes[i] = pairCoherent
		}
	}
	return classes, mixed, coherent, valid
}

func scoreFilmPattern(classes []int, mixed, valid int) (score float64, phase, expected, hits int) {
	if valid < 5 || mixed == 0 {
		return 0, 0, 0, 0
	}
	mixedRatio := float64(mixed) / float64(valid)
	best := -1.0
	for p := 0; p < 5; p++ {
		var exp, hit, miss, falseMixed int
		for i, class := range classes {
			want := (i-p+10)%5 == 0 || (i-p+10)%5 == 1
			if class == pairNeutral {
				continue
			}
			if want {
				exp++
				if class == pairMixed {
					hit++
				} else {
					miss++
				}
			} else if class == pairMixed {
				falseMixed++
			}
		}
		den := exp + hit + falseMixed
		if den == 0 {
			continue
		}
		f1 := 2 * float64(hit) / float64(den)
		cycles := float64(exp) / 2
		coverage := clamp(cycles/2, 0.35, 1)
		value := f1 * coverage
		switch {
		case mixedRatio > 0.55:
			value *= 0.10
		case mixedRatio < 0.15:
			value *= 0.20
		}
		if value > best {
			best = value
			phase = p
			expected = exp
			hits = hit
		}
	}
	if best < 0 {
		best = 0
	}
	return best, phase, expected, hits
}

func countTrue(values []bool) int {
	var n int
	for _, v := range values {
		if v {
			n++
		}
	}
	return n
}

func medianPair(pairs []pairMotion, within bool) float64 {
	values := make([]float64, 0, len(pairs))
	for _, pair := range pairs {
		if within {
			values = append(values, pair.within)
		} else if pair.hasAcross {
			values = append(values, pair.across)
		}
	}
	return median(values)
}

func staticPairRatio(pairs []pairMotion, floor float64) float64 {
	if len(pairs) == 0 {
		return 0
	}
	var still int
	var total int
	for _, pair := range pairs {
		if !pair.hasAcross {
			continue
		}
		total++
		if pair.across < floor {
			still++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(still) / float64(total)
}

func countPairCuts(pairs []pairMotion, threshold float64) int {
	var cuts int
	for _, pair := range pairs {
		if pair.hasAcross && pair.across >= threshold {
			cuts++
		}
	}
	return cuts
}

func pairSmoothness(pairs []pairMotion) float64 {
	values := make([]float64, 0, len(pairs))
	for _, pair := range pairs {
		if pair.hasAcross && pair.across < defaultCutThreshold {
			values = append(values, pair.across)
		}
	}
	return crossMotionSmoothness(values)
}

func pairWithinContrast(pairs []pairMotion) float64 {
	values := make([]float64, 0, len(pairs))
	for _, pair := range pairs {
		if pair.hasAcross && pair.across >= defaultStillFloor && pair.across < defaultCutThreshold {
			values = append(values, pair.within)
		}
	}
	if len(values) < 5 {
		return 0
	}
	med := median(values)
	spread := percentile(values, 0.85) - percentile(values, 0.15)
	return spread / (med + 0.0002)
}

func staticRatio(gaps []float64, floor float64) float64 {
	if len(gaps) == 0 {
		return 0
	}
	var n int
	for _, gap := range gaps {
		if gap < floor {
			n++
		}
	}
	return float64(n) / float64(len(gaps))
}

func countCuts(gaps []float64, threshold float64) int {
	var n int
	for _, gap := range gaps {
		if gap >= threshold {
			n++
		}
	}
	return n
}

func gapSmoothness(gaps []float64, duplicates []bool) float64 {
	var kept []float64
	for i, gap := range gaps {
		if i < len(duplicates) && duplicates[i] {
			continue
		}
		if gap >= defaultCutThreshold {
			continue
		}
		kept = append(kept, gap)
	}
	if len(kept) < 3 {
		return 0.5
	}
	med := median(kept)
	if med <= 1e-9 {
		return 1
	}
	var deviations []float64
	for _, v := range kept {
		deviations = append(deviations, math.Abs(v-med))
	}
	dispersion := median(deviations) / med
	return clamp(1-dispersion, 0, 1)
}

func pairedConcordance(within, across []float64) float64 {
	n := min(len(within), len(across))
	if n == 0 {
		return 0
	}
	var good int
	for i := 0; i < n; i++ {
		if across[i] > 1.35*within[i] && across[i] > defaultStillFloor {
			good++
		}
	}
	return float64(good) / float64(n)
}

func crossMotionSmoothness(gaps []float64) float64 {
	if len(gaps) < 3 {
		return 0.5
	}
	var filtered []float64
	for _, gap := range gaps {
		if gap < defaultCutThreshold {
			filtered = append(filtered, gap)
		}
	}
	if len(filtered) < 3 {
		return 0.5
	}
	med := median(filtered)
	if med <= 1e-9 {
		return 1
	}
	cv := stddev(filtered) / med
	return clamp(1-cv/1.5, 0, 1)
}

func orderContinuity(s fieldSequence) float64 {
	if len(s.fields) < 5 {
		return 0.5
	}
	var positive, total int
	prev := make([]float64, len(s.fields[0].desc.values))
	velocity := make([]float64, len(prev))
	havePrev := false
	for i := 1; i < len(s.fields); i++ {
		a := s.fields[i-1].desc.values
		b := s.fields[i].desc.values
		var mag float64
		for j := range b {
			velocity[j] = b[j] - a[j]
			mag += velocity[j] * velocity[j]
		}
		if !havePrev || mag < 1 {
			copy(prev, velocity)
			havePrev = mag >= 1
			continue
		}
		var dot, ma, mb float64
		for j := range velocity {
			dot += prev[j] * velocity[j]
			ma += prev[j] * prev[j]
			mb += velocity[j] * velocity[j]
		}
		if ma > 1 && mb > 1 {
			total++
			if dot > 0 {
				positive++
			}
		}
		copy(prev, velocity)
	}
	if total < 3 {
		return 0.5
	}
	return float64(positive) / float64(total)
}
