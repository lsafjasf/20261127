package main

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type Rational struct {
	Num int64
	Den int64
}

func parseRational(raw string) (Rational, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Rational{}, fmt.Errorf("empty rational")
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '/'
	})
	if len(parts) == 1 {
		value, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return Rational{}, fmt.Errorf("parse rational %q: %w", raw, err)
		}
		return Rational{Num: value, Den: 1}, nil
	}
	if len(parts) != 2 {
		return Rational{}, fmt.Errorf("invalid rational %q", raw)
	}
	num, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Rational{}, fmt.Errorf("parse numerator in %q: %w", raw, err)
	}
	den, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Rational{}, fmt.Errorf("parse denominator in %q: %w", raw, err)
	}
	if den <= 0 {
		return Rational{}, fmt.Errorf("denominator must be positive in %q", raw)
	}
	return Rational{Num: num, Den: den}, nil
}

func (r Rational) valid() bool {
	return r.Num > 0 && r.Den > 0
}

func (r Rational) String() string {
	if r.Den == 1 || r.Den == 0 {
		return strconv.FormatInt(r.Num, 10)
	}
	return strconv.FormatInt(r.Num, 10) + " " + strconv.FormatInt(r.Den, 10)
}

func (r Rational) rate() *big.Rat {
	return big.NewRat(r.Num, r.Den)
}

func (r Rational) unitsToSeconds(units int64) *big.Rat {
	if !r.valid() {
		return new(big.Rat)
	}
	return new(big.Rat).Mul(big.NewRat(units, 1), new(big.Rat).Inv(r.rate()))
}

func rationalFrame(seconds *big.Rat, rate Rational) *big.Rat {
	if !rate.valid() {
		return nil
	}
	return new(big.Rat).Mul(seconds, rate.rate())
}

func formatRat(value *big.Rat, digits int) string {
	if value == nil {
		return ""
	}
	return value.FloatString(digits)
}

func timecode(seconds *big.Rat, fps int64) string {
	if fps <= 0 || seconds == nil || seconds.Sign() < 0 {
		return ""
	}
	totalFrames := new(big.Rat).Mul(seconds, big.NewRat(fps, 1))
	if !totalFrames.IsInt() {
		return ""
	}
	frameCount := totalFrames.Num().Int64() / totalFrames.Denom().Int64()
	frames := frameCount % fps
	secondsCount := frameCount / fps
	secs := secondsCount % 60
	minutes := (secondsCount / 60) % 60
	hours := secondsCount / 3600
	return fmt.Sprintf("%02d:%02d:%02d:%02d", hours, minutes, secs, frames)
}
