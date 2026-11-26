package tailtrace

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Rational is an exact rational time value. Den is always positive after
// construction or JSON decoding.
type Rational struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

func NewRational(num, den int64) (Rational, error) {
	if den == 0 {
		if num == 0 {
			return Rational{Num: 0, Den: 1}, nil
		}
		return Rational{}, errors.New("rational denominator is zero")
	}
	if den < 0 {
		num, den = -num, -den
	}
	g := gcd64(num, den)
	return Rational{Num: num / g, Den: den / g}, nil
}

func MustRational(num, den int64) Rational {
	r, err := NewRational(num, den)
	if err != nil {
		panic(err)
	}
	return r
}

func ZeroRational() Rational { return Rational{Num: 0, Den: 1} }

func gcd64(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func (r Rational) normalize() Rational {
	if r.Den == 0 && r.Num == 0 {
		return ZeroRational()
	}
	nr, err := NewRational(r.Num, r.Den)
	if err != nil {
		panic(err)
	}
	return nr
}

func (r Rational) Add(other Rational) Rational {
	r = r.normalize()
	other = other.normalize()
	return MustRational(r.Num*other.Den+other.Num*r.Den, r.Den*other.Den)
}

func (r Rational) Sub(other Rational) Rational {
	r = r.normalize()
	other = other.normalize()
	return MustRational(r.Num*other.Den-other.Num*r.Den, r.Den*other.Den)
}

func (r Rational) Mul(other Rational) Rational {
	r = r.normalize()
	other = other.normalize()
	g1 := gcd64(r.Num, other.Den)
	g2 := gcd64(other.Num, r.Den)
	return MustRational((r.Num/g1)*(other.Num/g2), (r.Den/g2)*(other.Den/g1))
}

func (r Rational) Div(other Rational) Rational {
	return r.Mul(Rational{Num: other.Den, Den: other.Num})
}

func (r Rational) Cmp(other Rational) int {
	r = r.normalize()
	other = other.normalize()
	left := big.NewInt(r.Num)
	right := big.NewInt(other.Num)
	left.Mul(left, big.NewInt(r.Den))
	right.Mul(right, big.NewInt(other.Den))
	return left.Cmp(right)
}

func (r Rational) Float64() float64 {
	r = r.normalize()
	return float64(r.Num) / float64(r.Den)
}

func (r Rational) String() string {
	r = r.normalize()
	return fmt.Sprintf("%d/%d", r.Num, r.Den)
}

func (r Rational) SecondsText(digits int) string {
	r = r.normalize()
	return strconv.FormatFloat(r.Float64(), 'f', digits, 64) + "s"
}

func (r Rational) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *Rational) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	var object struct {
		Num int64 `json:"num"`
		Den int64 `json:"den"`
	}
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal(data, &object); err != nil {
			return err
		}
		parsed, perr := NewRational(object.Num, object.Den)
		if perr != nil {
			return perr
		}
		*r = parsed
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		parsed, perr := ParseRational(text)
		if perr != nil {
			return perr
		}
		*r = parsed
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		parsed, perr := ParseRational(number.String())
		if perr != nil {
			return perr
		}
		*r = parsed
		return nil
	}
	return fmt.Errorf("invalid rational: %s", string(data))
}

func ParseRational(text string) (Rational, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ZeroRational(), nil
	}
	if strings.Contains(text, "/") {
		parts := strings.Split(text, "/")
		if len(parts) != 2 {
			return Rational{}, fmt.Errorf("invalid rational %q", text)
		}
		num, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			return Rational{}, fmt.Errorf("invalid rational numerator %q", text)
		}
		den, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return Rational{}, fmt.Errorf("invalid rational denominator %q", text)
		}
		return NewRational(num, den)
	}
	if strings.ContainsAny(text, ".eE") {
		rat, ok := new(big.Rat).SetString(text)
		if !ok {
			return Rational{}, fmt.Errorf("invalid decimal rational %q", text)
		}
		return NewRational(rat.Num().Int64(), rat.Denom().Int64())
	}
	whole, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return Rational{}, fmt.Errorf("invalid rational %q", text)
	}
	return NewRational(whole, 1)
}
