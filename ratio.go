package aperturemap

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Ratio is an exact rational number. The zero value means 0/1.
type Ratio struct {
	N int64 `json:"n"`
	D int64 `json:"d"`
}

var ErrZeroDenominator = errors.New("ratio denominator must not be zero")

// NewRatio returns a reduced ratio.
func NewRatio(numerator, denominator int64) (Ratio, error) {
	if denominator == 0 {
		return Ratio{}, ErrZeroDenominator
	}
	r := big.NewRat(numerator, denominator)
	return Ratio{N: r.Num().Int64(), D: r.Denom().Int64()}, nil
}

// MustRatio is intended for compile-time-known fixtures and tests.
func MustRatio(numerator, denominator int64) Ratio {
	r, err := NewRatio(numerator, denominator)
	if err != nil {
		panic(err)
	}
	return r
}

// ParseRatio accepts "16/9", "16:9", "1.777777" and integer values.
func ParseRatio(text string) (Ratio, error) {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, ":", "/")
	if text == "" {
		return Ratio{N: 0, D: 1}, nil
	}
	var r big.Rat
	if _, ok := r.SetString(text); !ok {
		return Ratio{}, fmt.Errorf("invalid ratio %q", text)
	}
	return Ratio{N: r.Num().Int64(), D: r.Denom().Int64()}, nil
}

func (r Ratio) canonical() Ratio {
	if r.D == 0 && r.N == 0 {
		return Ratio{N: 0, D: 1}
	}
	return r
}

func (r Ratio) rat() *big.Rat {
	r = r.canonical()
	return big.NewRat(r.N, r.D)
}

func fromBigRat(r *big.Rat) Ratio {
	return Ratio{N: r.Num().Int64(), D: r.Denom().Int64()}
}

func (r Ratio) String() string {
	r = r.canonical()
	if r.D == 1 {
		return fmt.Sprintf("%d", r.N)
	}
	return fmt.Sprintf("%d/%d", r.N, r.D)
}

func (r Ratio) Float64() float64 {
	f, _ := r.rat().Float64()
	return f
}

func (r Ratio) IsZero() bool {
	return r.N == 0
}

func (r Ratio) Positive() bool {
	r = r.canonical()
	return r.N > 0 && r.D > 0
}

func (r Ratio) Cmp(other Ratio) int {
	return r.rat().Cmp(other.canonical().rat())
}

func (r Ratio) Equal(other Ratio) bool {
	return r.Cmp(other) == 0
}

func (r Ratio) Mul(other Ratio) Ratio {
	return fromBigRat(new(big.Rat).Mul(r.rat(), other.canonical().rat()))
}

func (r Ratio) Div(other Ratio) Ratio {
	o := other.canonical()
	return fromBigRat(new(big.Rat).Quo(r.rat(), o.rat()))
}

func (r Ratio) Add(other Ratio) Ratio {
	return fromBigRat(new(big.Rat).Add(r.rat(), other.canonical().rat()))
}

func (r Ratio) Sub(other Ratio) Ratio {
	return fromBigRat(new(big.Rat).Sub(r.rat(), other.canonical().rat()))
}

// ScaleInt returns the exact product of r and an integer.
func (r Ratio) ScaleInt(v int64) Ratio {
	return r.Mul(MustRatio(v, 1))
}

func (r Ratio) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *Ratio) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		parsed, perr := ParseRatio(text)
		if perr != nil {
			return perr
		}
		*r = parsed
		return nil
	}

	var object struct {
		N int64 `json:"n"`
		D int64 `json:"d"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("ratio must be a string or {n,d} object: %w", err)
	}
	parsed, err := NewRatio(object.N, object.D)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}
