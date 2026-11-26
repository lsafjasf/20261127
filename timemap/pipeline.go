package timemap

import "math/big"

// EditResolution is the integer timebase of the editor's edit clock, expressed
// as ticks per second. Edit timestamps are whole tick numbers.
type EditResolution struct {
	TicksPerSecond int64
}

// TickToSeconds converts an edit tick into exact edit time in seconds.
func (e EditResolution) TickToSeconds(tick int64) *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(tick), big.NewInt(e.TicksPerSecond))
}

// Stage records one clock domain crossing for one event.
type Stage struct {
	Index    *big.Int // integral unit index (frame number, sample number, slot); nil at edit stage
	Time     *big.Rat // exact time in seconds represented by that index
	Rule     RoundingRule
	Rate     Rate
	IsInt    bool // true when the scaled index needed no rounding
	FloatVal float64
}

// BugMode switches between the exact reference pipeline and the float64
// "as built" pipeline under test.
type BugMode int

const (
	ModeReference BugMode = iota // exact math/big.Rat, floor
	ModeAsBuilt                  // float64 + the track's configured buggy rule/rate
)

// TrackConfig describes one track (video / audio / subtitle) and the way the
// as-built pipeline is suspected to mishandle it.
type TrackConfig struct {
	Name        string
	MediaRate   Rate // true media frame/sample rate per second
	DisplayRule RoundingRule

	// As-built deviations; zero values mean "no deviation".
	BuiltMediaRate   *Rate        // wrong rate used by the buggy media stage
	BuiltMediaRule   RoundingRule // rounding used by the buggy media stage
	BuiltDisplayRule RoundingRule // rounding used by the buggy display stage
	BuiltUsesFloat   bool         // as-built path performs the media scaling in float64
	BuiltIndexBias   int64        // constant 1-based/0-based offset added at the media stage
}

// TimelinePoint is one absolute edit-tick instant reported by all tracks.
type TimelinePoint struct {
	Label string
	Tick  int64
}

// Event is the full three-stage crossing for one track at one edit point.
type Event struct {
	Track   string
	Point   TimelinePoint
	Edit    Stage
	Media   Stage
	Display Stage
}

// EditStage maps an edit tick onto the edit clock domain.
func EditStage(edit EditResolution, tick int64) Stage {
	t := edit.TickToSeconds(tick)
	return Stage{
		Index:    big.NewInt(tick),
		Time:     t,
		Rule:     RoundFloor,
		Rate:     NewRateInt(edit.TicksPerSecond),
		IsInt:    true,
		FloatVal: float64(tick) / float64(edit.TicksPerSecond),
	}
}

// MediaStage crosses edit time into the media clock (frame/sample indices).
func MediaStage(cfg TrackConfig, mode BugMode, editTime *big.Rat) Stage {
	rate := cfg.MediaRate
	rule := RoundFloor
	if mode == ModeAsBuilt {
		if cfg.BuiltMediaRate != nil {
			rate = *cfg.BuiltMediaRate
		}
		rule = cfg.BuiltMediaRule
	}

	if mode == ModeAsBuilt && cfg.BuiltUsesFloat {
		fv := float64Val(rate) * r2f(editTime)
		idx, isInt := floatQuantize(fv, rule)
		idx += bias(cfg, mode)
		return Stage{
			Index:    big.NewInt(idx),
			Time:     IndexTime(big.NewInt(idx), rate),
			Rule:     rule,
			Rate:     rate,
			IsInt:    isInt,
			FloatVal: fv,
		}
	}

	scaled := rate.Scale(editTime)
	idx := Quantize(scaled, rule)
	idx.Add(idx, big.NewInt(bias(cfg, mode)))
	return Stage{
		Index:    idx,
		Time:     IndexTime(idx, rate),
		Rule:     rule,
		Rate:     rate,
		IsInt:    scaled.IsInt(),
		FloatVal: r2f(scaled),
	}
}

func bias(cfg TrackConfig, mode BugMode) int64 {
	if mode == ModeAsBuilt {
		return cfg.BuiltIndexBias
	}
	return 0
}

// DisplayStage crosses media time onto the output display clock (vsync slots).
func DisplayStage(cfg TrackConfig, mode BugMode, refresh Rate, mediaTime *big.Rat) Stage {
	rule := cfg.DisplayRule
	if mode == ModeAsBuilt && cfg.BuiltDisplayRule != RoundFloor {
		rule = cfg.BuiltDisplayRule
	}

	if mode == ModeAsBuilt && cfg.BuiltUsesFloat {
		fv := float64Val(refresh) * r2f(mediaTime)
		idx, isInt := floatQuantize(fv, rule)
		return Stage{
			Index:    big.NewInt(idx),
			Time:     IndexTime(big.NewInt(idx), refresh),
			Rule:     rule,
			Rate:     refresh,
			IsInt:    isInt,
			FloatVal: fv,
		}
	}

	scaled := refresh.Scale(mediaTime)
	idx := Quantize(scaled, rule)
	return Stage{
		Index:    idx,
		Time:     IndexTime(idx, refresh),
		Rule:     rule,
		Rate:     refresh,
		IsInt:    scaled.IsInt(),
		FloatVal: r2f(scaled),
	}
}

// RunEvent computes the complete reference or as-built crossing for a track.
func RunEvent(cfg TrackConfig, mode BugMode, edit EditResolution, refresh Rate, p TimelinePoint) Event {
	ed := EditStage(edit, p.Tick)
	med := MediaStage(cfg, mode, ed.Time)
	disp := DisplayStage(cfg, mode, refresh, med.Time)
	return Event{Track: cfg.Name, Point: p, Edit: ed, Media: med, Display: disp}
}

func r2f(r *big.Rat) float64 {
	f, _ := r.Float64()
	return f
}

func float64Val(rate Rate) float64 {
	return r2f(rate.PerSecond())
}
