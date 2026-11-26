package tearcheck

import (
	"fmt"
	"math"
	"sort"
)

const (
	autoScoreThreshold   = 0.68
	autoMarginThreshold  = 0.10
	highScoreThreshold   = 0.82
	highMarginThreshold  = 0.18
	conflictScoreFloor   = 0.62
	reportCandidateFloor = 0.30
)

type component struct {
	name   string
	weight float64
	value  float64
	detail string
}

// Clamp01 constrains a score to [0,1].
func Clamp01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func normalizeObservation(o Observation) Observation {
	o.IntraFrameTear = Clamp01(o.IntraFrameTear)
	o.SeamCoverage = Clamp01(o.SeamCoverage)
	o.AdjacentFrameMatch = Clamp01(o.AdjacentFrameMatch)
	o.MatchContrast = Clamp01(o.MatchContrast)
	o.TimingFault = Clamp01(o.TimingFault)
	o.ArbitraryScanline = Clamp01(o.ArbitraryScanline)
	o.BlockAlignment = Clamp01(o.BlockAlignment)
	o.BlockArtifact = Clamp01(o.BlockArtifact)
	o.TemporalBlockRepeat = Clamp01(o.TemporalBlockRepeat)
	o.CodecError = Clamp01(o.CodecError)
	o.ContentMarker = Clamp01(o.ContentMarker)
	o.ChromaEffect = Clamp01(o.ChromaEffect)
	o.EffectCoherence = Clamp01(o.EffectCoherence)
	if o.RepeatedSide == "" {
		o.RepeatedSide = SideNone
	}
	return o
}

func weightedScore(parts []component) (float64, []Evidence) {
	var score float64
	evidence := make([]Evidence, 0, len(parts))
	for _, part := range parts {
		value := Clamp01(part.value)
		score += part.weight * value
		evidence = append(evidence, Evidence{
			Name:   part.name,
			Weight: part.weight,
			Value:  round3(value),
			Detail: part.detail,
		})
	}
	return Clamp01(score), evidence
}

// IsCandidate reports whether an observation contains enough anomaly to retain.
func IsCandidate(o Observation) bool {
	o = normalizeObservation(o)
	strongest := maxFloat(
		o.IntraFrameTear,
		o.AdjacentFrameMatch,
		o.TimingFault,
		o.CodecError,
		o.ContentMarker,
	)
	if strongest >= reportCandidateFloor {
		return true
	}
	if o.BlockArtifact >= 0.55 && o.BlockAlignment >= 0.55 {
		return true
	}
	return o.ChromaEffect >= 0.75 && o.IntraFrameTear >= 0.35
}

// ClassifyObservation maps normalized evidence to an explainable verdict.
// Weak or mutually contradictory evidence becomes CauseManual.
func ClassifyObservation(in Observation) Finding {
	o := normalizeObservation(in)

	hardwareParts := []component{
		{name: "adjacent_frame_match", weight: 0.32, value: o.AdjacentFrameMatch, detail: "one side of the intra-frame split matches an adjacent frame"},
		{name: "match_contrast", weight: 0.12, value: o.MatchContrast, detail: "matched side is substantially closer than the other side"},
		{name: "intra_frame_fault", weight: 0.22, value: o.IntraFrameTear, detail: "horizontal discontinuity or displaced continuation inside one frame"},
		{name: "seam_coverage", weight: 0.08, value: o.SeamCoverage, detail: "fraction of width supporting the scanline split"},
		{name: "arbitrary_scanline", weight: 0.14, value: o.ArbitraryScanline, detail: "seam is scanline-like and not tied to a codec grid"},
		{name: "capture_timing_fault", weight: 0.12, value: o.TimingFault, detail: "capture interval or capture/PTS clock discontinuity"},
	}
	encodingParts := []component{
		{name: "codec_or_decoder_error", weight: 0.38, value: o.CodecError, detail: "bitstream error, packet loss, or decoder concealment"},
		{name: "coding_block_alignment", weight: 0.18, value: o.BlockAlignment, detail: "fault aligns with a 4/8/16/32/64-pixel codec boundary"},
		{name: "block_artifact", weight: 0.22, value: o.BlockArtifact, detail: "block-grid energy, damaged macroblocks, or missing residuals"},
		{name: "temporal_block_repeat", weight: 0.22, value: o.TemporalBlockRepeat, detail: "damaged/reference-reconstructed blocks persist across frames"},
	}
	contentParts := []component{
		{name: "trusted_content_marker", weight: 0.42, value: o.ContentMarker, detail: "game log, scene marker, or effect mask identifies intentional effect"},
		{name: "chroma_effect_style", weight: 0.24, value: o.ChromaEffect, detail: "saturated color split or stylized glitch palette"},
		{name: "effect_coherence", weight: 0.19, value: o.EffectCoherence, detail: "multiple bands move coherently with designed effect geometry"},
		{name: "no_framebuffer_splice", weight: 0.15, value: 1 - o.AdjacentFrameMatch, detail: "no side behaves like an exact previous/next framebuffer fragment"},
	}

	hardwareScore, hardwareEvidence := weightedScore(hardwareParts)
	encodingScore, encodingEvidence := weightedScore(encodingParts)
	contentScore, contentEvidence := weightedScore(contentParts)

	// Strong contradictory provenance explains why a superficial signal is
	// present and prevents two hypotheses from remaining artificially tied.
	hardwareFactor := 1.0
	if o.CodecError >= 0.75 && o.BlockAlignment >= 0.65 {
		hardwareFactor *= 0.42
	}
	if o.ContentMarker >= 0.85 && o.AdjacentFrameMatch < 0.55 {
		hardwareFactor *= 0.55
	}
	if o.AdjacentFrameMatch >= 0.85 && o.MatchContrast >= 0.65 &&
		o.IntraFrameTear >= 0.55 && o.BlockAlignment < 0.55 && o.CodecError < 0.4 {
		encodingScore *= 0.55
	}
	hardwareScore *= hardwareFactor

	// Strong direct provenance can resolve an otherwise ambiguous visual seam.
	if o.CodecError >= 0.85 && o.BlockAlignment >= 0.65 {
		encodingScore = math.Max(encodingScore, 0.84)
	}
	if o.ContentMarker >= 0.9 && o.AdjacentFrameMatch < 0.55 {
		contentScore = math.Max(contentScore, 0.84)
	}
	if o.ChromaEffect >= 0.8 && o.EffectCoherence >= 0.75 &&
		o.AdjacentFrameMatch <= 0.3 && o.BlockAlignment < 0.55 {
		contentScore = math.Max(contentScore, 0.78)
	}
	if o.AdjacentFrameMatch >= 0.85 && o.MatchContrast >= 0.65 && o.IntraFrameTear >= 0.55 &&
		!(o.CodecError >= 0.75 && o.BlockAlignment >= 0.65) {
		hardwareScore = math.Max(hardwareScore, 0.84)
	}

	scores := map[Cause]float64{
		CauseHardwareSync: round3(hardwareScore),
		CauseEncoding:     round3(encodingScore),
		CauseContent:      round3(contentScore),
	}
	type ranked struct {
		cause Cause
		score float64
	}
	ranking := []ranked{
		{CauseHardwareSync, hardwareScore},
		{CauseEncoding, encodingScore},
		{CauseContent, contentScore},
	}
	sort.Slice(ranking, func(i, j int) bool { return ranking[i].score > ranking[j].score })
	top := ranking[0]
	second := ranking[1]
	margin := round3(top.score - second.score)

	evidence := make([]Evidence, 0, len(hardwareEvidence)+len(encodingEvidence)+len(contentEvidence)+2)
	evidence = append(evidence, Evidence{Name: "hardware_score", Weight: 1, Value: round3(hardwareScore)})
	evidence = append(evidence, hardwareEvidence...)
	evidence = append(evidence, Evidence{Name: "encoding_score", Weight: 1, Value: round3(encodingScore)})
	evidence = append(evidence, encodingEvidence...)
	evidence = append(evidence, Evidence{Name: "content_score", Weight: 1, Value: round3(contentScore)})
	evidence = append(evidence, contentEvidence...)

	conflicting := hardwareScore >= conflictScoreFloor && encodingScore >= conflictScoreFloor ||
		hardwareScore >= conflictScoreFloor && contentScore >= conflictScoreFloor ||
		encodingScore >= conflictScoreFloor && contentScore >= conflictScoreFloor

	finding := Finding{
		Cause:          top.cause,
		Score:          round3(top.score),
		Margin:         maxFloat(0, margin),
		Scores:         scores,
		Evidence:       evidence,
		FrameIndex:     o.FrameIndex,
		Time:           o.Time,
		SeamY:          o.SeamY,
		ReviewRequired: true,
	}

	reasons := make([]string, 0, 4)
	switch {
	case top.score < autoScoreThreshold:
		reasons = append(reasons, fmt.Sprintf("top score %.2f is below auto threshold %.2f", top.score, autoScoreThreshold))
	case top.score >= highScoreThreshold && margin >= highMarginThreshold && !conflicting:
		finding.Confidence = ConfidenceHigh
		finding.ReviewRequired = false
		reasons = append(reasons, "strong evidence and clear margin over alternatives")
	case top.score >= autoScoreThreshold && margin >= autoMarginThreshold && !conflicting:
		finding.Confidence = ConfidenceMedium
		finding.ReviewRequired = false
		reasons = append(reasons, "enough concordant evidence for medium-confidence classification")
	default:
		reasons = append(reasons, fmt.Sprintf("insufficient separation: margin %.2f, conflicting=%v", margin, conflicting))
	}

	if finding.ReviewRequired {
		finding.Cause = CauseManual
		finding.Confidence = ConfidenceLow
		possible := make([]Cause, 0, 3)
		manualScores := make(map[Cause]float64, 3)
		for _, item := range ranking {
			if item.score >= 0.30 || len(possible) < 2 {
				possible = append(possible, item.cause)
				manualScores[item.cause] = round3(item.score)
			}
		}
		finding.Manual = &ManualItem{
			Reason:         joinReasons(reasons, o.Messages),
			PossibleCauses: possible,
			Scores:         manualScores,
			Observation:    o,
		}
	}
	return finding
}

func joinReasons(reasons, messages []string) string {
	all := append(append([]string{}, reasons...), messages...)
	if len(all) == 0 {
		return "low-confidence visual event"
	}
	out := ""
	for i, reason := range all {
		if i > 0 {
			out += "; "
		}
		out += reason
	}
	return out
}

func maxFloat(values ...float64) float64 {
	best := 0.0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
