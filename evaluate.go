package laddercheck

import "fmt"

// EvaluateLadder builds the unified constraint and classifies every rung.
func EvaluateLadder(inputName string, facts []RenditionFacts, opts ConstraintOptions) *Report {
	c := GenerateConstraint(facts, opts)
	r := &Report{Schema: SchemaVersion, Input: inputName, Constraint: c, Facts: facts}
	for i := range facts {
		r.Verdicts = append(r.Verdicts, evaluateOne(facts[i], c))
	}
	r.Summary = buildSummary(r.Verdicts)
	return r
}

func buildSummary(vs []Verdict) Summary {
	s := Summary{Renditions: len(vs), SwitchSafe: true}
	for _, v := range vs {
		id := verdictID(v)
		switch v.Status {
		case StatusCompliant:
			s.Compliant = append(s.Compliant, id)
		case StatusRemuxOnly:
			s.RemuxOnly = append(s.RemuxOnly, id)
			s.SwitchSafe = false
		case StatusReencode:
			s.RequiresReencode = append(s.RequiresReencode, id)
			s.SwitchSafe = false
		default:
			s.Insufficient = append(s.Insufficient, id)
			s.SwitchSafe = false
		}
	}
	s.AlignableWithoutReencode = len(s.RequiresReencode) == 0 && len(s.Insufficient) == 0
	return s
}

func verdictID(v Verdict) string {
	if v.Role != "" {
		return v.ID + "(" + v.Role + ")"
	}
	return v.ID
}

type matchedGrid struct {
	j         int64
	contentT  Rat // unified content-relative time j*gop
	absoluteT Rat // origin + contentT on the display timeline
	kf        KeyframeFact
}

func evaluateOne(f RenditionFacts, c KeyframeConstraint) Verdict {
	v := Verdict{
		ID:                   f.ID,
		Role:                 f.Role,
		FirstDisplayPTS:      f.FirstDisplayPTS,
		GOPUniform:           f.GOPUniform,
		FPSCompatible:        true,
		AllKeyframesIDR:      true,
		MaxDisplayError:      ZeroRat(),
		MaxMediaCadenceError: ZeroRat(),
	}

	if f.SampleCount == 0 || len(f.Keyframes) == 0 {
		v.Status = StatusInsufficient
		v.Reasons = append(v.Reasons, "insufficient_evidence: no video samples/keyframes parsed")
		v.Actions = append(v.Actions, "supply a parseable ISO-BMFF file or keyframe evidence JSON")
		return v
	}

	if f.VariableFrameDuration {
		v.FPSCompatible = false
		v.Reasons = append(v.Reasons, "variable_frame_rate: unified grid assumes CFR")
	}
	if c.FPS.N > 0 && f.FPS.N > 0 && c.FPS.Cmp(f.FPS) != 0 {
		v.FPSCompatible = false
		v.Reasons = append(v.Reasons, fmt.Sprintf("fps_mismatch: rung=%s constraint=%s", f.FPS, c.FPS))
	}

	// Anchor content time at the first presented media PTS. Subtracting this
	// removes timescale/origin/initial-PTS labels, leaving the true GOP layout.
	anchor := f.EarliestPTS
	if anchor.N == 0 && anchor.D == 0 && f.FirstPTS.N != 0 {
		anchor = f.FirstPTS
	}

	frameDur := f.FrameDuration
	if frameDur.N == 0 && f.FPS.N > 0 {
		frameDur = NewRat(f.FPS.D, f.FPS.N)
	}
	lastFrameStart := ZeroRat()
	if frameDur.N > 0 && f.SampleCount > 0 {
		lastFrameStart = frameDur.Mul(NewRat(int64(f.SampleCount-1), 1))
	} else if len(f.Keyframes) > 0 && c.GOPSeconds.N > 0 {
		lastKF := f.Keyframes[len(f.Keyframes)-1].PTS.Sub(anchor)
		lastFrameStart = lastKF
	}

	type gridPoint struct {
		contentT, absoluteT Rat
		j                   int64
	}
	var grid []gridPoint
	if c.GOPSeconds.N > 0 {
		for j := int64(0); ; j++ {
			ct := NewRat(j, 1).Mul(c.GOPSeconds)
			if ct.Cmp(lastFrameStart.Add(c.ToleranceSeconds)) > 0 {
				break
			}
			grid = append(grid, gridPoint{j: j, contentT: ct, absoluteT: c.OriginSeconds.Add(ct)})
			if j > 1_000_000 {
				break
			}
		}
	}
	v.GridKeyframeCount = len(grid)

	var hard, nonIDRSamples, unknownIDRSamples []string
	var matches []matchedGrid
	maxErr := ZeroRat()
	var delta0 Rat
	haveDelta := false
	maxSpread := ZeroRat()

	for _, gp := range grid {
		kf, derr, ok := nearestContentKeyframe(f.Keyframes, anchor, gp.contentT)
		if !ok || derr.Cmp(c.ToleranceSeconds) > 0 {
			v.MissingGridTimes = append(v.MissingGridTimes, gp.absoluteT)
			hard = append(hard, fmt.Sprintf("no_idr_grid_point:content_t=%s display_t=%s nearest_err=%s",
				gp.contentT, gp.absoluteT, derr))
			continue
		}
		matches = append(matches, matchedGrid{j: gp.j, contentT: gp.contentT, absoluteT: gp.absoluteT, kf: kf})
		v.MatchedCount++

		dispErr := kf.DisplayPTS.Sub(gp.absoluteT)
		if a := dispErr.Abs(); a.Cmp(maxErr) > 0 {
			maxErr = a
		}
		if dispErr.Abs().Cmp(c.ToleranceSeconds) > 0 {
			v.OffGridTimes = append(v.OffGridTimes, kf.DisplayPTS)
		}
		if !haveDelta {
			delta0 = dispErr
			haveDelta = true
		} else if sp := dispErr.Sub(delta0).Abs(); sp.Cmp(maxSpread) > 0 {
			maxSpread = sp
		}

		if c.RequireIDR {
			switch {
			case kf.IDR == nil:
				unknownIDRSamples = append(unknownIDRSamples, fmt.Sprintf("%d@t=%s", kf.Sample, gp.contentT))
			case !*kf.IDR:
				nonIDRSamples = append(nonIDRSamples, fmt.Sprintf("%d@t=%s", kf.Sample, gp.contentT))
				v.NonIDRSamples = append(v.NonIDRSamples, kf.Sample)
				v.AllKeyframesIDR = false
			}
		}
	}

	v.MaxDisplayError = maxErr
	if haveDelta {
		d := delta0
		v.LabelOffsetSeconds = &d
	}
	for _, d := range f.GOPMediaDurations {
		if c.GOPSeconds.N > 0 {
			if e := d.Sub(c.GOPSeconds).Abs(); e.Cmp(v.MaxMediaCadenceError) > 0 {
				v.MaxMediaCadenceError = e
			}
		}
	}

	v.ContentAligned = len(v.MissingGridTimes) == 0 && len(matches) > 0
	v.SamplesAligned = decodeIndicesAligned(f, c.GOPFrames)
	if v.ContentAligned && !v.SamplesAligned && c.GOPFrames > 0 {
		v.Reasons = append(v.Reasons,
			"decode_order_indices_differ (B-frame reorder depth), but PTS/content grid aligns; safe for timeline-based remux assessment")
	}

	if G := c.GOPFrames; G > 0 {
		var extras []int
		for _, k := range f.Keyframes {
			if int64(k.Sample)%G != 0 {
				extras = append(extras, k.Sample)
			}
		}
		if len(extras) > 0 && v.ContentAligned {
			v.Reasons = append(v.Reasons, fmt.Sprintf("extra_or_reordered_keyframe_samples:%v (advisory)", extras))
		}
	}

	displayAligned := v.ContentAligned && maxErr.Cmp(c.ToleranceSeconds) <= 0
	constantOffset := maxSpread.Cmp(c.ToleranceSeconds) <= 0
	v.DisplayAligned = displayAligned

	if !v.FPSCompatible {
		hard = append(hard, "frame_rate_incompatible_with_unified_cfr_grid")
	}
	if len(nonIDRSamples) > 0 {
		hard = append(hard, "non_idr_at_grid:"+joinComma(nonIDRSamples)+" (open-GOP/CRA cannot become closed IDR via remux)")
	}
	if v.ContentAligned && !constantOffset {
		hard = append(hard, "non_constant_timing_drift: display offset is not a single edit-list/PTS shift")
	}

	classify(&v, f, c, hard, unknownIDRSamples, matches, delta0, displayAligned)
	return v
}

// nearestContentKeyframe finds the keyframe whose media PTS is closest to the
// content-grid point anchor+contentT. PTS is presentation order, so this is
// immune to B-frame decode reordering.
func nearestContentKeyframe(ks []KeyframeFact, anchor, contentT Rat) (KeyframeFact, Rat, bool) {
	target := anchor.Add(contentT)
	best := -1
	var bestD Rat
	for i, k := range ks {
		d := k.PTS.Sub(target).Abs()
		if best == -1 || d.Cmp(bestD) < 0 {
			best, bestD = i, d
		}
	}
	if best == -1 {
		return KeyframeFact{}, ZeroRat(), false
	}
	return ks[best], bestD, true
}

func decodeIndicesAligned(f RenditionFacts, gopFrames int64) bool {
	if gopFrames <= 0 || f.SampleCount == 0 {
		return false
	}
	set := map[int]bool{}
	for _, k := range f.Keyframes {
		set[k.Sample] = true
	}
	for j := int64(0); j*gopFrames < int64(f.SampleCount); j++ {
		if !set[int(j*gopFrames)] {
			return false
		}
	}
	return true
}

func classify(v *Verdict, f RenditionFacts, c KeyframeConstraint, hard, unknown []string,
	matches []matchedGrid, delta0 Rat, displayAligned bool) {
	switch {
	case len(unknown) > 0 && len(hard) == 0:
		v.Status = StatusInsufficient
		v.RemuxPossible = false
		v.Reasons = append(v.Reasons, "idr_unverified_for_samples:"+joinComma(unknown))
		v.Actions = append(v.Actions, "supply decodable samples/avcC-hvcC or idr flags so closed-IDR status can be certified")
	case len(hard) > 0:
		v.Status = StatusReencode
		v.RemuxPossible = false
		v.Reasons = append(v.Reasons, hard...)
		v.Actions = append(v.Actions, reencodeActions(c)...)
	default:
		if displayAligned {
			v.Status = StatusCompliant
			v.RemuxPossible = true
			v.Reasons = append(v.Reasons, "aligned: closed-IDR content grid, GOP and display times match the unified constraint")
			v.Actions = append(v.Actions, "none; preserve zero origin PTS and the normalized edit list")
			return
		}
		v.Status = StatusRemuxOnly
		v.RemuxPossible = true
		refMediaPTS := f.FirstPTS
		if len(matches) > 0 {
			refMediaPTS = matches[0].kf.PTS
		}
		v.Reasons = append(v.Reasons,
			fmt.Sprintf("container_label_offset_only: content/PTS IDR grid is correct; display grid is shifted by %ss", delta0),
			"gop_idr_structure_ok: correction is PTS/edit-list metadata, no coded samples change")
		v.Actions = append(v.Actions, remuxActions(f, c, delta0, refMediaPTS)...)
	}
}

func joinComma(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func reencodeActions(c KeyframeConstraint) []string {
	a := []string{
		"re-encode this rung: no closed IDR exists at a unified content/switch point (missing grid point, GOP cadence/phase, or non-IDR CRA/open-GOP)",
	}
	if c.GOPFrames > 0 && c.FPS.N > 0 {
		a = append(a, fmt.Sprintf(
			"encode at %s CFR with fixed GOP %d frames (%s s), keyint=min-keyint=%d, scenecut=0, closed GOP/IDR",
			c.FPS, c.GOPFrames, c.GOPSeconds, c.GOPFrames))
	} else {
		a = append(a, "encode with the unified fixed GOP and closed IDR settings")
	}
	a = append(a, c.EncoderHints...)
	return a
}

func remuxActions(f RenditionFacts, c KeyframeConstraint, offset, refMediaPTS Rat) []string {
	minus := offset.Mul(NewRat(-1, 1))
	return []string{
		fmt.Sprintf("metadata-only fix (no re-encode): apply a constant %+ss display shift so the reference IDR reaches %ss",
			minus.String(), c.OriginSeconds),
		fmt.Sprintf("edit-list contract: one rate-1 edit at movie start %ss with media_time=%ss (the reference IDR media PTS); this maps display=media-%s and absorbs the %ss residual",
			c.OriginSeconds, refMediaPTS, refMediaPTS, offset),
		"equivalently shift all PTS by " + minus.String() + "s and strip rung-specific empty edits; preserve every coded sample",
		"apply with an MP4 box/timestamp rewrite or stream-copy remux, then re-run laddercheck to confirm compliant",
		"ffmpeg representative: ffmpeg -i " + orDash(f.Path, "in.mp4") + " -map 0 -c copy -avoid_negative_ts make_zero " + orDash(f.ID, "rung") + ".aligned.mp4  (verify exact elst/PTS result)",
	}
}

func orDash(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
