package timemap

import (
	"fmt"
	"math/big"
)

// StageName identifies one of the three clock-domain crossings.
type StageName string

const (
	StageEdit    StageName = "edit"
	StageMedia   StageName = "media"
	StageDisplay StageName = "display"
)

// Cause classifies the root of a detected one-frame mismatch.
type Cause string

const (
	CauseNone      Cause = "none"
	CauseRounding  Cause = "rounding-rule"
	CauseWrongRate Cause = "wrong-frame-rate"
	CauseOffByOne  Cause = "off-by-one-indexing"
	CauseCompound  Cause = "compound-rate-and-rounding"
	CauseUnknown   Cause = "unknown"
)

// Finding is the verdict for one track.
type Finding struct {
	Track           string
	Stage           StageName
	Cause           Cause
	FirstDivergence *TimelinePoint
	DeltaAtFirst    *big.Int // as-built index minus reference index
	DivergentPoints int
	TotalPoints     int
	DriftAtLast     *big.Int
	Detail          string
}

// CauseCN returns the Chinese label of the classified cause.
func (f Finding) CauseCN() string {
	switch f.Cause {
	case CauseNone:
		return "无偏差"
	case CauseRounding:
		return "舍入规则错误（应为 floor，实际用了四舍五入）"
	case CauseWrongRate:
		return "帧率/采样率错误"
	case CauseOffByOne:
		return "索引 0 基/1 基错位"
	case CauseCompound:
		return "帧率与舍入复合错误"
	default:
		return "未知原因"
	}
}

// Locate compares the as-built events against the reference events and
// pinpoints the first clock-domain crossing where they disagree, then tests
// exact arithmetic hypotheses to name the responsible code step.
func Locate(cfg TrackConfig, edit EditResolution, refresh Rate, points []TimelinePoint, ref, built []Event) Finding {
	f := Finding{Track: cfg.Name, TotalPoints: len(points)}

	for i := range points {
		if ref[i].Edit.Index.Cmp(built[i].Edit.Index) != 0 {
			f.Stage = StageEdit
			f.Cause = CauseUnknown
			f.FirstDivergence = &points[i]
			f.DeltaAtFirst = delta(built[i].Edit.Index, ref[i].Edit.Index)
			f.Detail = "edit tick decode diverged; check edit timebase configuration"
			return f
		}
	}

	firstMedia := -1
	for i := range points {
		if ref[i].Media.Index.Cmp(built[i].Media.Index) != 0 {
			firstMedia = i
			break
		}
	}

	if firstMedia >= 0 {
		f.Stage = StageMedia
		f.FirstDivergence = &points[firstMedia]
		f.DeltaAtFirst = delta(built[firstMedia].Media.Index, ref[firstMedia].Media.Index)
		f.Cause, f.Detail = classifyMedia(cfg, edit, points, built)
		f.DivergentPoints, f.DriftAtLast = divergenceStats(points, ref, built, StageMedia)
		return f
	}

	firstDisplay := -1
	for i := range points {
		if ref[i].Display.Index.Cmp(built[i].Display.Index) != 0 {
			firstDisplay = i
			break
		}
	}
	if firstDisplay >= 0 {
		f.Stage = StageDisplay
		f.FirstDivergence = &points[firstDisplay]
		f.DeltaAtFirst = delta(built[firstDisplay].Display.Index, ref[firstDisplay].Display.Index)
		f.Cause, f.Detail = classifyDisplay(cfg, refresh, points, built)
		f.DivergentPoints, f.DriftAtLast = divergenceStats(points, ref, built, StageDisplay)
		return f
	}

	f.Cause = CauseNone
	f.Detail = "as-built pipeline matches the exact reference at every stage and point"
	return f
}

func classifyMedia(cfg TrackConfig, edit EditResolution, points []TimelinePoint, built []Event) (Cause, string) {
	if offByOneAtOrigin(built, func(e Event) *big.Int { return e.Media.Index }) {
		return CauseOffByOne, "media index is already shifted at edit time 0: 1-based frame/sample numbering used where 0-based is required"
	}

	rateDiffers := cfg.BuiltMediaRate != nil && cfg.BuiltMediaRate.PerSecond().Cmp(cfg.MediaRate.PerSecond()) != 0
	ruleDiffers := cfg.BuiltMediaRule != RoundFloor

	roundingMatch := true
	rateMatch := true
	for i, p := range points {
		editTime := edit.TickToSeconds(p.Tick)
		roundingIdx := Quantize(cfg.MediaRate.Scale(editTime), cfg.BuiltMediaRule)
		if roundingIdx.Cmp(built[i].Media.Index) != 0 {
			roundingMatch = false
		}
		if rateDiffers {
			rateIdx := Quantize(cfg.BuiltMediaRate.Scale(editTime), RoundFloor)
			if rateIdx.Cmp(built[i].Media.Index) != 0 {
				rateMatch = false
			}
		} else {
			rateMatch = false
		}
	}

	switch {
	case roundingMatch && rateMatch:
		if rateDiffers && ruleDiffers {
			return CauseCompound, fmt.Sprintf("media stage uses rate %s instead of %s and rounds %s instead of floor",
				rateLabel(*cfg.BuiltMediaRate), rateLabel(cfg.MediaRate), cfg.BuiltMediaRule)
		}
	case roundingMatch:
		return CauseRounding, fmt.Sprintf("media stage quantizes with %s at rate %s; floor at the same rate reproduces every as-built index after correction",
			cfg.BuiltMediaRule, rateLabel(cfg.MediaRate))
	case rateMatch:
		return CauseWrongRate, fmt.Sprintf("media stage scales time at %s instead of the true media rate %s",
			rateLabel(*cfg.BuiltMediaRate), rateLabel(cfg.MediaRate))
	}

	switch {
	case rateDiffers && ruleDiffers:
		return CauseCompound, fmt.Sprintf("media stage combines wrong rate %s (want %s) and %s rounding",
			rateLabel(*cfg.BuiltMediaRate), rateLabel(cfg.MediaRate), cfg.BuiltMediaRule)
	case rateDiffers:
		return CauseWrongRate, fmt.Sprintf("media stage scales time at %s instead of the true media rate %s",
			rateLabel(*cfg.BuiltMediaRate), rateLabel(cfg.MediaRate))
	case ruleDiffers:
		return CauseRounding, fmt.Sprintf("media stage quantizes with %s instead of floor", cfg.BuiltMediaRule)
	default:
		return CauseUnknown, "media indices diverge but neither a rate nor a rounding hypothesis reproduces them"
	}
}

func classifyDisplay(cfg TrackConfig, refresh Rate, points []TimelinePoint, built []Event) (Cause, string) {
	if offByOneAtOrigin(built, func(e Event) *big.Int { return e.Display.Index }) {
		return CauseOffByOne, "display slot is shifted at time 0: first vsync slot numbered from 1 instead of 0"
	}
	ruleDiffers := cfg.BuiltDisplayRule != RoundFloor
	roundingMatch := true
	for i := range points {
		idx := Quantize(refresh.Scale(built[i].Media.Time), cfg.BuiltDisplayRule)
		if idx.Cmp(built[i].Display.Index) != 0 {
			roundingMatch = false
		}
	}
	if ruleDiffers && roundingMatch {
		return CauseRounding, fmt.Sprintf("display stage schedules vsync slots with %s instead of floor at %s",
			cfg.BuiltDisplayRule, rateLabel(refresh))
	}
	return CauseUnknown, "display slots diverge but no configured hypothesis reproduces them"
}

func offByOneAtOrigin(built []Event, pick func(Event) *big.Int) bool {
	if len(built) == 0 || built[0].Point.Tick != 0 {
		return false
	}
	return pick(built[0]).Cmp(big.NewInt(0)) != 0
}

func divergenceStats(points []TimelinePoint, ref, built []Event, stage StageName) (int, *big.Int) {
	count := 0
	var drift *big.Int
	for i := range points {
		var r, b *big.Int
		switch stage {
		case StageMedia:
			r, b = ref[i].Media.Index, built[i].Media.Index
		case StageDisplay:
			r, b = ref[i].Display.Index, built[i].Display.Index
		default:
			r, b = ref[i].Edit.Index, built[i].Edit.Index
		}
		if r.Cmp(b) != 0 {
			count++
			drift = delta(b, r)
		}
	}
	return count, drift
}

func delta(built, ref *big.Int) *big.Int {
	return new(big.Int).Sub(built, ref)
}

func rateLabel(r Rate) string {
	return r.PerSecond().RatString()
}
