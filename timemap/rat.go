package timemap

import (
	"fmt"
	"math/big"
	"strings"
)

// RoundingRule selects how a continuous media time is reduced to an
// integral unit index (frame number, sample number, display slot).
type RoundingRule int

const (
	RoundFloor   RoundingRule = iota // reference: slot n owns [n/rate, (n+1)/rate)
	RoundNearest                     // common bug: math.Round pushes a boundary up one unit
	RoundCeil
)

func (r RoundingRule) String() string {
	switch r {
	case RoundFloor:
		return "floor"
	case RoundNearest:
		return "nearest"
	case RoundCeil:
		return "ceil"
	default:
		return "unknown"
	}
}

// ParseRounding maps the textual rule used in configs/tests.
func ParseRounding(s string) (RoundingRule, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "floor":
		return RoundFloor, nil
	case "nearest", "round":
		return RoundNearest, nil
	case "ceil":
		return RoundCeil, nil
	default:
		return RoundFloor, fmt.Errorf("unknown rounding rule %q", s)
	}
}

// Rate is an exact frame/sample/refresh rate as a rational number per second.
type Rate struct {
	num *big.Rat
}

// NewRate builds rate num/den units per second (e.g. 24000/1001 fps).
func NewRate(num, den int64) Rate {
	if den <= 0 {
		panic("rate denominator must be positive")
	}
	return Rate{num: big.NewRat(num, den)}
}

// NewRateInt builds an integral rate (e.g. 48000 Hz).
func NewRateInt(n int64) Rate {
	return Rate{num: big.NewRat(n, 1)}
}

// PerSecond returns the rate as an exact rational.
func (r Rate) PerSecond() *big.Rat {
	return new(big.Rat).Set(r.num)
}

// Scale converts a time in seconds to an exact (fractional) unit index.
func (r Rate) Scale(t *big.Rat) *big.Rat {
	return new(big.Rat).Mul(t, r.num)
}

// Quantize reduces a fractional unit index to an integral unit index.
func Quantize(x *big.Rat, rule RoundingRule) *big.Int {
	switch rule {
	case RoundFloor:
		return new(big.Int).Div(x.Num(), x.Denom())
	case RoundCeil:
		z := new(big.Int).Quo(x.Num(), x.Denom())
		rem := new(big.Int).Rem(x.Num(), x.Denom())
		if rem.Sign() != 0 {
			z.Add(z, big.NewInt(1))
		}
		return z
	case RoundNearest:
		// round half away from zero, matching math.Round semantics
		num := new(big.Int).Set(x.Num())
		den := new(big.Int).Set(x.Denom())
		neg := num.Sign() < 0
		if neg {
			num.Neg(num)
		}
		q := new(big.Int).Quo(num, den)
		twoRem := new(big.Int).Mul(new(big.Int).Rem(num, den), big.NewInt(2))
		if twoRem.Cmp(den) >= 0 {
			q.Add(q, big.NewInt(1))
		}
		if neg {
			q.Neg(q)
		}
		return q
	default:
		panic("unreachable")
	}
}

// IndexTime converts an integral unit index back into the exact media
// timestamp represented by that index.
func IndexTime(index *big.Int, rate Rate) *big.Rat {
	return new(big.Rat).SetFrac(new(big.Int).Set(index), rate.num.Num())
}

// ratInt builds an exact rational from an int64.
func ratInt(n int64) *big.Rat {
	return big.NewRat(n, 1)
}

// FormatSeconds prints a *big.Rat second value as a fixed-point decimal with
// exactly prec digits, without any floating-point conversion.
func FormatSeconds(r *big.Rat, prec int) string {
	if r == nil {
		return ""
	}
	sign := ""
	if r.Sign() < 0 {
		sign = "-"
		r = new(big.Rat).Neg(r)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(prec)), nil)
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(scale))
	whole := Quantize(scaled, RoundFloor)
	wholeStr := whole.String()
	if len(wholeStr) <= prec {
		wholeStr = strings.Repeat("0", prec+1-len(wholeStr)) + wholeStr
	}
	intPart := wholeStr[:len(wholeStr)-prec]
	fracPart := wholeStr[len(wholeStr)-prec:]
	return sign + intPart + "." + fracPart
}
