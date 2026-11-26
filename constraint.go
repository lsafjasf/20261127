package laddercheck

import (
	"fmt"
	"sort"
)

// GenerateConstraint derives the unified keyframe grid from options and the
// observed rungs. An explicit GOP/FPS in opts always wins; otherwise the
// modal uniform GOP and common FPS are inferred.
func GenerateConstraint(facts []RenditionFacts, opts ConstraintOptions) KeyframeConstraint {
	c := KeyframeConstraint{
		Schema:           SchemaVersion,
		OriginSeconds:    zeroOr(opts.OriginSeconds, ZeroRat()),
		RequireIDR:       true,
		ClosedGOP:        true,
		AlignTimeline:    "display_pts_after_edit_list",
		EditListPolicy:   "single identity-mapped edit (or no elst); no rung-specific empty edits / media_time offsets",
		FirstFramePolicy: "first presented frame and reference IDR at the unified origin (PTS/display start == origin)",
	}
	if opts.RequireIDR != nil {
		c.RequireIDR = *opts.RequireIDR
	}

	// Common frame rate.
	switch {
	case opts.FPS.N > 0:
		c.FPS = opts.FPS
	default:
		c.FPS = commonFPS(facts)
	}

	// Unified GOP duration / frame count.
	switch {
	case opts.GOPSeconds.N > 0:
		c.GOPSeconds = opts.GOPSeconds
		c.Source = "explicit:gop_seconds"
	case opts.GOPFrames > 0 && c.FPS.N > 0:
		c.GOPFrames = opts.GOPFrames
		c.GOPSeconds = NewRat(opts.GOPFrames*c.FPS.D, c.FPS.N)
		c.Source = "explicit:gop_frames+fps"
	default:
		if g, frames, fps, ok := inferGOP(facts); ok {
			c.GOPSeconds = g
			if c.FPS.N == 0 && fps.N > 0 {
				c.FPS = fps
			}
			if c.FPS.N > 0 {
				if k := ratFrames(g, c.FPS); k > 0 && g.Equal(NewRat(k*c.FPS.D, c.FPS.N)) {
					c.GOPFrames = k
				}
			}
			if frames > 0 {
				c.GOPFrames = frames
			}
			c.Source = "inferred:modal-uniform-gop"
		} else {
			c.GOPSeconds = NewRat(2, 1)
			c.Source = "default:2s"
		}
	}

	if opts.GOPFrames > 0 {
		c.GOPFrames = opts.GOPFrames
		if c.FPS.N > 0 {
			c.GOPSeconds = NewRat(opts.GOPFrames*c.FPS.D, c.FPS.N)
		}
	}
	if c.GOPFrames == 0 && c.FPS.N > 0 && c.GOPSeconds.N > 0 {
		if k := ratFrames(c.GOPSeconds, c.FPS); k > 0 &&
			c.GOPSeconds.Equal(NewRat(k*c.FPS.D, c.FPS.N)) {
			c.GOPFrames = k
		}
	}

	// Tolerance defaults to half a frame, else 1 ms.
	switch {
	case opts.ToleranceSeconds.N > 0:
		c.ToleranceSeconds = opts.ToleranceSeconds
	case c.FPS.N > 0:
		frameDur := NewRat(c.FPS.D, c.FPS.N)
		c.ToleranceSeconds = frameDur.Quo(NewRat(2, 1))
	default:
		c.ToleranceSeconds = NewRat(1, 1000)
	}

	if opts.HorizonSeconds.N > 0 && c.GOPSeconds.N > 0 {
		h := opts.HorizonSeconds
		for t := c.OriginSeconds; t.Cmp(c.OriginSeconds.Add(h)) <= 0; t = t.Add(c.GOPSeconds) {
			c.GridKeyframeTimes = append(c.GridKeyframeTimes, t)
		}
	}

	c.EncoderHints = encoderHints(c)
	return c
}

func zeroOr(v, def Rat) Rat {
	if v.N == 0 && v.D == 0 {
		return def
	}
	return v
}

func commonFPS(facts []RenditionFacts) Rat {
	count := map[string]int{}
	var order []Rat
	for _, f := range facts {
		if f.FPS.N <= 0 {
			continue
		}
		key := f.FPS.String()
		if _, ok := count[key]; !ok {
			order = append(order, f.FPS)
		}
		count[key]++
	}
	if len(order) == 0 {
		return ZeroRat()
	}
	sort.SliceStable(order, func(i, j int) bool {
		return count[order[i].String()] > count[order[j].String()]
	})
	return order[0]
}

// inferGOP picks the modal uniform GOP observed across rungs.
func inferGOP(facts []RenditionFacts) (gop Rat, frames int64, fps Rat, ok bool) {
	type cand struct {
		gop   Rat
		count int
	}
	var cands []cand
	idx := map[string]int{}
	for _, f := range facts {
		if !f.GOPUniform || len(f.GOPMediaDurations) == 0 {
			continue
		}
		g := f.GOPMediaDurations[0]
		key := g.String()
		if i, exists := idx[key]; exists {
			cands[i].count++
		} else {
			idx[key] = len(cands)
			cands = append(cands, cand{gop: g, count: 1})
		}
		if fps.N == 0 && f.FPS.N > 0 {
			fps = f.FPS
		}
	}
	if len(cands) == 0 {
		return Rat{}, 0, Rat{}, false
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].count != cands[j].count {
			return cands[i].count > cands[j].count
		}
		return cands[i].gop.Cmp(cands[j].gop) < 0
	})
	g := cands[0].gop
	var k int64
	if fps.N > 0 {
		k = ratFrames(g, fps)
	}
	return g, k, fps, true
}

// ratFrames returns round(gop*fps) only when it is exact enough to be an
// integer frame count; -1 otherwise.
func ratFrames(gop, fps Rat) int64 {
	product := gop.Mul(fps)
	rf := product.Float64()
	k := int64(roundAway(rf))
	if k <= 0 {
		return -1
	}
	if product.Sub(NewRat(k, 1)).Abs().Cmp(NewRat(1, 1_000_000)) <= 0 {
		return k
	}
	return -1
}

func encoderHints(c KeyframeConstraint) []string {
	g := c.GOPFrames
	if g == 0 && c.FPS.N > 0 {
		g = ratFrames(c.GOPSeconds, c.FPS)
	}
	hints := []string{
		"common: identical CFR frame rate and identical GOP length measured in frames across every rung",
		"common: first coded frame is a closed IDR at frame index 0; force an IDR at every unified grid point",
		"common: disable scene-cut / adaptive keyframe insertion so extra frames cannot shift the GOP cadence",
		"container: normalize start PTS to the origin and make elst identical (or strip rung-specific edit lists)",
	}
	if c.FPS.N > 0 {
		hints = append(hints, fmt.Sprintf("ffmpeg video: -r %s -fps_mode cfr", c.FPS.String()))
	}
	if g > 0 {
		hints = append(hints,
			fmt.Sprintf("libx264: -g %d -keyint_min %d -sc_threshold 0 -x264-params keyint=%d:min-keyint=%d:scenecut=0", g, g, g, g),
			fmt.Sprintf("libx265: -x265-params keyint=%d:min-keyint=%d:scenecut=0:no-open-gop=1", g, g),
		)
	}
	if c.GOPSeconds.N > 0 {
		hints = append(hints, fmt.Sprintf(
			"forced IDR times (optional, time based): -force_key_frames \"expr:gte(t,n_forced*%s)\"",
			c.GOPSeconds.String()))
	}
	hints = append(hints, "verify: laddercheck analyze <rungs...> must report every rung compliant or remux_only_fixable")
	return hints
}
