package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

// DB is a dBFS level. Negative infinity (no signal at all) marshals to JSON
// null instead of the illegal token -Infinity.
type DB float64

func (d DB) MarshalJSON() ([]byte, error) {
	if math.IsInf(float64(d), -1) {
		return []byte("null"), nil
	}
	return json.Marshal(float64(d))
}

// Threshold names reported in every output line.
const (
	LevelDigital = "digital" // exact zero samples only
	LevelNoise   = "noise"   // low-level noise floor
	LevelPause   = "pause"   // quiet pause / breath gap
)

// Interval positions.
const (
	PosLeadIn   = "lead-in"
	PosInterior = "interior"
	PosTrailing = "trailing"
	PosWhole    = "whole-file"
)

// Verdict labels.
const (
	VerdictRemovable  = "removable"  // digital or pure noise floor, safe to trim
	VerdictAcceptable = "acceptable" // natural short pause
	VerdictReview     = "review"     // borderline long pause
	VerdictSuspect    = "suspect"    // unusually long pause, likely sync issue
	VerdictBoundary   = "boundary"   // head/tail silence, check file offsets
)

// Config controls detection sensitivity. All durations are milliseconds.
type Config struct {
	WindowMS  float64
	BridgeMS  float64
	MinDurMS  float64
	MicroMS   float64
	NaturalMS float64
	LongMS    float64
	UseRMS    bool
	NoiseDB   float64
	PauseDB   float64
}

// DefaultConfig returns sensible spoken-audio defaults.
func DefaultConfig() Config {
	return Config{
		WindowMS:  10,
		BridgeMS:  15,
		MinDurMS:  30,
		MicroMS:   200,
		NaturalMS: 800,
		LongMS:    2000,
		NoiseDB:   -60,
		PauseDB:   -45,
	}
}

// Validate rejects contradictory thresholds and durations. dBFS thresholds
// must get stricter (more negative) from pause to noise to digital.
func (c Config) Validate() error {
	if c.NoiseDB >= c.PauseDB {
		return fmt.Errorf("noise-db (%.1f) must be below pause-db (%.1f)",
			c.NoiseDB, c.PauseDB)
	}
	if c.WindowMS <= 0 || c.BridgeMS < 0 || c.MinDurMS < 0 {
		return errors.New("window-ms must be > 0 and durations must be >= 0")
	}
	if !(c.MicroMS <= c.NaturalMS && c.NaturalMS <= c.LongMS) {
		return errors.New("duration order must be micro-ms <= natural-ms <= long-ms")
	}
	return nil
}

// Interval is one detected quiet region. End is exclusive (frame indices).
type Interval struct {
	Channel    int     `json:"channel"` // -1 means the whole program (all channels)
	Level      string  `json:"level"`
	Start      int     `json:"startFrame"`
	End        int     `json:"endFrame"`
	StartSec   float64 `json:"startSec"`
	EndSec     float64 `json:"endSec"`
	DurationMS float64 `json:"durationMs"`
	PeakDB     DB      `json:"peakDbfs"`
	RMSDB      DB      `json:"rmsDbfs"`
	ZeroPct    float64 `json:"zeroPercent"`
	Position   string  `json:"position"`
	Kind       string  `json:"kind"`
	Verdict    string  `json:"verdict"`
}

// Detect classifies every quiet frame into exactly one tier (digital, noise
// floor, or pause), then emits one interval per contiguous tier for each
// channel and for the all-channel program view.
func Detect(a *Audio, cfg Config) []*Interval {
	var out []*Interval
	for ch := 0; ch < a.Channels; ch++ {
		out = append(out, detectOne(a, ch, cfg)...)
	}
	out = append(out, detectOne(a, -1, cfg)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return levelRank(out[i].Level) < levelRank(out[j].Level)
		}
		if out[i].Channel != out[j].Channel {
			return out[i].Channel < out[j].Channel
		}
		return out[i].Start < out[j].Start
	})
	return out
}

func levelRank(l string) int {
	switch l {
	case LevelDigital:
		return 0
	case LevelNoise:
		return 1
	default:
		return 2
	}
}

func detectOne(a *Audio, ch int, cfg Config) []*Interval {
	mags, zeros := collect(a, ch)

	noiseLin := int64(math.Round(math.Pow(10, cfg.NoiseDB/20) * (1 << 31)))
	pauseLin := int64(math.Round(math.Pow(10, cfg.PauseDB/20) * (1 << 31)))

	// Per-window tier: 0=content, 1=pause, 2=noise, 3=digital.
	rate := a.SampleRate
	win := max(1, int(math.Round(cfg.WindowMS/1000*float64(rate))))
	n := len(mags)
	tiers := make([]uint8, 0, n/win+1)
	for off := 0; off < n; off += win {
		end := min(off+win, n)
		var peak int64
		allZero := true
		if cfg.UseRMS {
			var sumSq float64
			for i := off; i < end; i++ {
				v := float64(mags[i])
				sumSq += v * v
				if !zeros[i] {
					allZero = false
				}
			}
			peak = int64(math.Sqrt(sumSq / float64(end-off)))
		} else {
			for i := off; i < end; i++ {
				if mags[i] > peak {
					peak = mags[i]
				}
				if !zeros[i] {
					allZero = false
				}
			}
		}
		switch {
		case allZero:
			tiers = append(tiers, 3)
		case peak <= noiseLin:
			tiers = append(tiers, 2)
		case peak <= pauseLin:
			tiers = append(tiers, 1)
		default:
			tiers = append(tiers, 0)
		}
	}

	// Bridge short content windows caused by clicks, then merge same-tier runs.
	bridgeWindows := max(1, int(math.Ceil((cfg.BridgeMS/1000*float64(rate))/float64(win))))
	tiers = bridgeClicks(tiers, bridgeWindows)

	var out []*Interval
	start := 0
	for i := 1; i <= len(tiers); i++ {
		if i < len(tiers) && tiers[i] == tiers[start] {
			continue
		}
		tier := tiers[start]
		s := start * win
		e := min(i*win, n)
		start = i
		if tier == 0 {
			continue
		}
		s, e = refineEdges(mags, zeros, s, e, tier, noiseLin, pauseLin)
		iv := buildInterval(a, ch, tierName(tier), s, e, cfg)
		if iv != nil {
			out = append(out, iv)
		}
	}
	return out
}

// collect returns per-frame magnitudes and zero flags for one channel, or
// the all-channel program view (peak magnitude, quiet only if every channel
// is zero) when ch is negative.
func collect(a *Audio, ch int) ([]int64, []bool) {
	n := a.Frames
	mags := make([]int64, n)
	zeros := make([]bool, n)
	for i := 0; i < n; i++ {
		if ch >= 0 {
			s := a.Samples[ch][i]
			mags[i] = int64(abs32(s))
			zeros[i] = s == 0
			continue
		}
		allZero := true
		var peak int64
		for c := 0; c < a.Channels; c++ {
			s := a.Samples[c][i]
			if s != 0 {
				allZero = false
			}
			if m := int64(abs32(s)); m > peak {
				peak = m
			}
		}
		mags[i] = peak
		zeros[i] = allZero
	}
	return mags, zeros
}

// bridgeClicks fills a run of content windows no longer than bridgeWindows
// when it is flanked on both sides by the same non-content tier.
func bridgeClicks(tiers []uint8, bridgeWindows int) []uint8 {
	out := append([]uint8(nil), tiers...)
	for i := 0; i < len(out); i++ {
		if out[i] != 0 {
			continue
		}
		j := i
		for j < len(out) && out[j] == 0 {
			j++
		}
		if i > 0 && j < len(out) && j-i <= bridgeWindows &&
			out[i-1] != 0 && out[i-1] == out[j] {
			for k := i; k < j; k++ {
				out[k] = out[i-1]
			}
		}
		i = j - 1
	}
	return out
}

// refineEdges expands a run to sample-accurate boundaries using per-frame
// tiers, so window quantization does not inflate durations.
func refineEdges(mags []int64, zeros []bool, start, end int,
	tier uint8, noiseLin, pauseLin int64) (int, int) {

	match := func(i int) bool {
		return frameTier(mags[i], zeros[i], noiseLin, pauseLin) == tier
	}
	for start > 0 && match(start-1) {
		start--
	}
	for end < len(mags) && match(end) {
		end++
	}
	return start, end
}

func frameTier(mag int64, zero bool, noiseLin, pauseLin int64) uint8 {
	switch {
	case zero:
		return 3
	case mag <= noiseLin:
		return 2
	case mag <= pauseLin:
		return 1
	default:
		return 0
	}
}

func tierName(tier uint8) string {
	switch tier {
	case 3:
		return LevelDigital
	case 2:
		return LevelNoise
	default:
		return LevelPause
	}
}

func buildInterval(a *Audio, ch int, level string, start, end int,
	cfg Config) *Interval {

	minFrames := int(math.Round(cfg.MinDurMS / 1000 * float64(a.SampleRate)))
	if end-start < minFrames {
		return nil
	}

	peak, rms, zeroPct := intervalStats(a, ch, start, end)
	iv := &Interval{
		Channel:    ch,
		Level:      level,
		Start:      start,
		End:        end,
		StartSec:   float64(start) / float64(a.SampleRate),
		EndSec:     float64(end) / float64(a.SampleRate),
		DurationMS: float64(end-start) / float64(a.SampleRate) * 1000,
		PeakDB:     toDBFS(peak),
		RMSDB:      toDBFS(rms),
		ZeroPct:    zeroPct,
	}
	iv.Position = positionOf(start, end, a.Frames)
	iv.Kind, iv.Verdict = classify(level, iv.DurationMS, iv.Position, zeroPct, cfg)
	return iv
}

func intervalStats(a *Audio, ch, start, end int) (peak, rms float64, zeroPct float64) {
	var sumSq float64
	var count, zeros int
	channels := []int{ch}
	if ch < 0 {
		channels = make([]int, a.Channels)
		for i := range channels {
			channels[i] = i
		}
	}
	for _, c := range channels {
		for i := start; i < end; i++ {
			s := a.Samples[c][i]
			v := float64(s)
			sumSq += v * v
			count++
			if s == 0 {
				zeros++
			}
			if av := math.Abs(v); av > peak {
				peak = av
			}
		}
	}
	rms = math.Sqrt(sumSq / float64(count))
	zeroPct = float64(zeros) / float64(count) * 100
	return peak, rms, zeroPct
}

func positionOf(start, end, total int) string {
	switch {
	case start == 0 && end == total:
		return PosWhole
	case start == 0:
		return PosLeadIn
	case end == total:
		return PosTrailing
	default:
		return PosInterior
	}
}

// classify turns raw duration/level facts into the user-facing labels that
// separate trimmable material from acceptable natural pauses.
func classify(level string, durMS float64, position string, zeroPct float64,
	cfg Config) (kind, verdict string) {

	if position != PosInterior {
		switch level {
		case LevelDigital:
			return "head/tail digital silence", VerdictBoundary
		case LevelNoise:
			return "head/tail noise floor", VerdictBoundary
		default:
			return "head/tail quiet", VerdictBoundary
		}
	}

	switch {
	case durMS < cfg.MicroMS:
		kind = "micro-gap"
	case durMS <= cfg.NaturalMS:
		kind = "natural pause"
	case durMS <= cfg.LongMS:
		kind = "long pause"
	default:
		kind = "extended pause"
	}

	switch level {
	case LevelDigital:
		kind = "digital silence"
		verdict = VerdictRemovable
	case LevelNoise:
		kind = "noise floor"
		verdict = VerdictRemovable
	default: // LevelPause
		switch kind {
		case "micro-gap", "natural pause":
			verdict = VerdictAcceptable
		case "long pause":
			verdict = VerdictReview
		default:
			verdict = VerdictSuspect
		}
	}
	return kind, verdict
}

func toDBFS(v float64) DB {
	if v <= 0 {
		return DB(math.Inf(-1))
	}
	return DB(20 * math.Log10(v/float64(1<<31)))
}

func abs32(x int32) int32 {
	if x == math.MinInt32 {
		return math.MaxInt32
	}
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
