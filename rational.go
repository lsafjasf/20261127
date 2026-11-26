package laddercheck

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
)

// Rat is an exact rational number. Den is always > 0 after construction or
// JSON decoding. The zero value is interpreted as 0/1.
type Rat struct {
	N int64 `json:"n"`
	D int64 `json:"d"`
}

func normRat(n, d int64) Rat {
	if d == 0 {
		d = 1
	}
	if d < 0 {
		n, d = -n, -d
	}
	g := gcd64(abs64(n), d)
	if g > 0 {
		n /= g
		d /= g
	}
	return Rat{N: n, D: d}
}

func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// NewRat returns the reduced fraction n/d.
func NewRat(n, d int64) Rat { return normRat(n, d) }

// ZeroRat is the rational zero.
func ZeroRat() Rat { return Rat{N: 0, D: 1} }

func (r Rat) den() int64 {
	if r.D == 0 {
		return 1
	}
	return r.D
}

func (r Rat) big() *big.Rat { return big.NewRat(r.N, r.den()) }

// ParseRat parses "30000/1001", "30", "0.5", a JSON number token, or "".
func ParseRat(s string) (Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ZeroRat(), nil
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		n, err1 := strconv.ParseInt(strings.TrimSpace(s[:i]), 10, 64)
		d, err2 := strconv.ParseInt(strings.TrimSpace(s[i+1:]), 10, 64)
		if err1 != nil || err2 != nil || d == 0 {
			return Rat{}, &RatError{Value: s}
		}
		return normRat(n, d), nil
	}
	if f, ok := new(big.Rat).SetString(s); ok {
		return bigToRat(f), nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return Rat{}, &RatError{Value: s}
	}
	return NewRat(n, 1), nil
}

// RatError reports an unparseable rational literal.
type RatError struct{ Value string }

func (e *RatError) Error() string { return "laddercheck: invalid rational " + strconv.Quote(e.Value) }

func bigToRat(f *big.Rat) Rat {
	return normRat(f.Num().Int64(), f.Denom().Int64())
}

func (r Rat) String() string {
	d := r.den()
	if d == 1 {
		return strconv.FormatInt(r.N, 10)
	}
	return strconv.FormatInt(r.N, 10) + "/" + strconv.FormatInt(d, 10)
}

// MarshalJSON emits the canonical "n" or "n/d" string.
func (r Rat) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// UnmarshalJSON accepts "n/d", "n", decimal strings, JSON numbers and null.
func (r *Rat) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" {
		*r = ZeroRat()
		return nil
	}
	if len(s) > 0 && s[0] == '"' {
		var lit string
		if err := json.Unmarshal(data, &lit); err != nil {
			return err
		}
		parsed, err := ParseRat(lit)
		if err != nil {
			return err
		}
		*r = parsed
		return nil
	}
	parsed, err := ParseRat(s)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// Float64 is intended for display only; comparisons stay exact.
func (r Rat) Float64() float64 {
	f, _ := r.big().Float64()
	return f
}

// Cmp returns -1/0/1 like big.Rat.Cmp.
func (r Rat) Cmp(o Rat) int { return r.big().Cmp(o.big()) }

// Equal reports exact rational equality.
func (r Rat) Equal(o Rat) bool { return r.Cmp(o) == 0 }

func (r Rat) Add(o Rat) Rat { return bigToRat(new(big.Rat).Add(r.big(), o.big())) }
func (r Rat) Sub(o Rat) Rat { return bigToRat(new(big.Rat).Sub(r.big(), o.big())) }
func (r Rat) Mul(o Rat) Rat { return bigToRat(new(big.Rat).Mul(r.big(), o.big())) }
func (r Rat) Quo(o Rat) Rat { return bigToRat(new(big.Rat).Quo(r.big(), o.big())) }

// Abs returns the absolute value.
func (r Rat) Abs() Rat {
	if r.N < 0 {
		return NewRat(-r.N, r.den())
	}
	return r
}

// IsInt reports whether the value is an integer.
func (r Rat) IsInt() bool { return r.N%r.den() == 0 }

// NumDen returns the reduced numerator and positive denominator.
func (r Rat) NumDen() (int64, int64) { return r.N, r.den() }

// RatFromTicks converts timescale ticks to seconds.
func RatFromTicks(ticks int64, timescale uint32) Rat {
	if timescale == 0 {
		return ZeroRat()
	}
	return NewRat(ticks, int64(timescale))
}

// GridDistance returns the signed distance of t to the nearest point of the
// grid origin+k*step and that nearest integer k. step must be > 0.
func GridDistance(t, origin, step Rat) (k int64, distance Rat) {
	ratio := t.Sub(origin).Quo(step)
	// Round ratio to nearest integer, halves go away from zero.
	rf := ratio.Float64()
	k = int64(roundAway(rf))
	nearest := origin.Add(step.Mul(NewRat(k, 1)))
	return k, t.Sub(nearest)
}

func roundAway(v float64) float64 {
	if v >= 0 {
		return float64(int64(v + 0.5))
	}
	return -float64(int64(-v + 0.5))
}

// AllClose reports whether every a[i]-b[i] (pairwise) is within tol.
func AllClose(a, b []Rat, tol Rat) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Sub(b[i]).Abs().Cmp(tol) > 0 {
			return false
		}
	}
	return true
}
