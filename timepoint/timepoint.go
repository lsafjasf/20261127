// Package timepoint parses and maps chapter timestamps without silently
// turning malformed or deleted points into zero.
package timepoint

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const NanosecondPerSecond uint64 = 1_000_000_000

type Status string

const (
	StatusResolved          Status = "resolved"
	StatusEmpty             Status = "time_empty"
	StatusUnparseable       Status = "time_unparseable"
	StatusNegative          Status = "time_negative"
	StatusFrameRateRequired Status = "frame_rate_required"
	StatusFrameRateInvalid  Status = "frame_rate_invalid"
	StatusFrameOutOfRange   Status = "frame_out_of_range"
	StatusRemoved           Status = "time_removed_by_edit"
	StatusBeforeZero        Status = "time_before_zero"
	StatusOutOfTargetRange  Status = "time_out_of_target_range"
	StatusTimelineConflict  Status = "timeline_conflict"
)

// Point is the only result returned by tolerant parsing. A non-resolved
// point must be listed in the migration report instead of being used as 0.
type Point struct {
	Raw    string  `json:"raw"`
	NS     *uint64 `json:"ns,omitempty"`
	Status Status  `json:"status"`
	Reason string  `json:"reason,omitempty"`
}

func Resolved(ns uint64) Point {
	return Point{Raw: FormatNS(ns), NS: &ns, Status: StatusResolved}
}

func Unresolved(raw string, status Status, reason string) Point {
	return Point{Raw: raw, Status: status, Reason: reason}
}

func (p Point) Ok() bool { return p.Status == StatusResolved && p.NS != nil }

// Rate is an exact frame rate Num/Den.
type Rate struct {
	Num uint64
	Den uint64
}

func ParseRate(raw string) (Rate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Rate{}, errors.New("empty frame rate")
	}
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		if len(parts) != 2 {
			return Rate{}, fmt.Errorf("invalid rational rate %q", raw)
		}
		num, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || num == 0 {
			return Rate{}, fmt.Errorf("invalid rate numerator %q", raw)
		}
		den, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || den == 0 {
			return Rate{}, fmt.Errorf("invalid rate denominator %q", raw)
		}
		return Rate{Num: num, Den: den}, nil
	}
	if strings.ContainsAny(raw, ".") {
		rat, ok := decimalToRational(raw)
		if !ok {
			return Rate{}, fmt.Errorf("invalid decimal rate %q", raw)
		}
		return rat, nil
	}
	num, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || num == 0 {
		return Rate{}, fmt.Errorf("invalid rate %q", raw)
	}
	return Rate{Num: num, Den: 1}, nil
}

func decimalToRational(raw string) (Rate, bool) {
	parts := strings.SplitN(raw, ".", 2)
	whole, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return Rate{}, false
	}
	frac := parts[1]
	if frac == "" {
		return Rate{Num: whole, Den: 1}, true
	}
	if len(frac) > 9 {
		frac = frac[:9]
	}
	n, err := strconv.ParseUint(frac, 10, 64)
	if err != nil {
		return Rate{}, false
	}
	den := uint64(math.Pow10(len(frac)))
	return Rate{Num: whole*den + n, Den: den}, true
}

type ParseOptions struct {
	FPS Rate
}

// Parse accepts seconds, MM:SS[.frac], HH:MM:SS[.frac], and HH:MM:SS:FF
// when an exact FPS is supplied. Native Matroska uint values should use
// FromNS because they are already nanoseconds.
func Parse(raw string, opts ...ParseOptions) Point {
	original := raw
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Unresolved(original, StatusEmpty, "timestamp is empty")
	}
	if strings.HasPrefix(raw, "-") {
		return Unresolved(original, StatusNegative, "negative timestamps are not supported")
	}
	fps := Rate{}
	if len(opts) > 0 {
		fps = opts[0].FPS
	}

	colonParts := strings.Split(raw, ":")
	switch len(colonParts) {
	case 1:
		sec, err := parseSeconds(colonParts[0])
		if err != nil {
			return Unresolved(original, StatusUnparseable, err.Error())
		}
		return Resolved(sec)
	case 2, 3:
		seconds, err := parseClock(colonParts)
		if err != nil {
			return Unresolved(original, StatusUnparseable, err.Error())
		}
		return Resolved(seconds)
	case 4:
		if fps.Num == 0 || fps.Den == 0 {
			return Unresolved(original, StatusFrameRateRequired,
				"frame-coded timestamp needs --fps or an explicit rate mapping")
		}
		seconds, err := parseClock(colonParts[:3])
		if err != nil {
			return Unresolved(original, StatusUnparseable, err.Error())
		}
		frames, err := strconv.ParseUint(strings.TrimSpace(colonParts[3]), 10, 64)
		if err != nil {
			return Unresolved(original, StatusUnparseable, fmt.Sprintf("invalid frame component %q", colonParts[3]))
		}
		if frames*fps.Den >= fps.Num {
			return Unresolved(original, StatusFrameOutOfRange,
				fmt.Sprintf("frame %d is outside rate %d/%d", frames, fps.Num, fps.Den))
		}
		frameNS := frames * NanosecondPerSecond * fps.Den / fps.Num
		return Resolved(seconds + frameNS)
	default:
		return Unresolved(original, StatusUnparseable, "too many colon components")
	}
}

func FromNS(raw string, ns uint64, valid bool) Point {
	if !valid {
		return Unresolved(raw, StatusUnparseable, "EBML timestamp value is absent or invalid")
	}
	return Resolved(ns)
}

func parseSeconds(raw string) (uint64, error) {
	whole, frac, err := splitFraction(raw)
	if err != nil {
		return 0, err
	}
	return whole*NanosecondPerSecond + frac, nil
}

func parseClock(parts []string) (uint64, error) {
	var hours uint64
	var minutes uint64
	secondPart := parts[0]
	if len(parts) == 2 {
		min, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minute component %q", parts[0])
		}
		minutes = min
		secondPart = parts[1]
	} else {
		hour, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hour component %q", parts[0])
		}
		min, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minute component %q", parts[1])
		}
		if min >= 60 {
			return 0, fmt.Errorf("minutes must be below 60 in HH:MM:SS")
		}
		hours, minutes, secondPart = hour, min, parts[2]
	}
	seconds, err := parseSeconds(secondPart)
	if err != nil {
		return 0, err
	}
	if seconds/NanosecondPerSecond >= 60 {
		return 0, fmt.Errorf("seconds must be below 60 in a clock timestamp")
	}
	return (hours*3600+minutes*60)*NanosecondPerSecond + seconds, nil
}

func splitFraction(raw string) (wholeNS uint64, fracNS uint64, err error) {
	raw = strings.TrimSpace(raw)
	sep := strings.IndexAny(raw, ".,")
	intPart := raw
	fracPart := ""
	if sep >= 0 {
		intPart = raw[:sep]
		fracPart = raw[sep+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	whole, err := strconv.ParseUint(intPart, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid seconds %q", raw)
	}
	if len(fracPart) > 9 {
		fracPart = fracPart[:9]
	}
	if fracPart != "" {
		if _, err := strconv.ParseUint(fracPart, 10, 64); err != nil {
			return 0, 0, fmt.Errorf("invalid fraction %q", raw)
		}
	}
	frac, _ := strconv.ParseUint(fracPart, 10, 64)
	frac *= uint64(math.Pow10(9 - len(fracPart)))
	return whole, frac, nil
}

func FormatNS(ns uint64) string {
	hours := ns / (3600 * NanosecondPerSecond)
	ns %= 3600 * NanosecondPerSecond
	minutes := ns / (60 * NanosecondPerSecond)
	ns %= 60 * NanosecondPerSecond
	seconds := ns / NanosecondPerSecond
	frac := ns % NanosecondPerSecond
	return fmt.Sprintf("%02d:%02d:%02d.%09d", hours, minutes, seconds, frac)
}
