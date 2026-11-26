package effectinterp

import (
	"fmt"
	"math"
)

// Kind identifies how a parameter value is interpolated.
type Kind uint8

const (
	KindNumber Kind = iota + 1
	KindAngle
	KindBoolean
	KindColor
)

func (k Kind) String() string {
	switch k {
	case KindNumber:
		return "number"
	case KindAngle:
		return "angle"
	case KindBoolean:
		return "boolean"
	case KindColor:
		return "color"
	default:
		return fmt.Sprintf("Kind(%d)", k)
	}
}

func (k Kind) valid() bool {
	return k >= KindNumber && k <= KindColor
}

// Color is a straight-alpha, 8-bit RGBA color. Components are interpolated
// independently in sRGB space.
type Color struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

// Value holds exactly one parameter value. Kind determines which field is
// meaningful; the constructors below set it consistently.
type Value struct {
	Kind    Kind
	Number  float64
	Boolean bool
	Color   Color
}

func NumberValue(v float64) Value {
	return Value{Kind: KindNumber, Number: v}
}

func AngleValue(degrees float64) Value {
	return Value{Kind: KindAngle, Number: degrees}
}

func BooleanValue(v bool) Value {
	return Value{Kind: KindBoolean, Boolean: v}
}

func ColorValue(c Color) Value {
	return Value{Kind: KindColor, Color: c}
}

// RGBA is a convenience constructor for a color value.
func RGBA(r, g, b, a uint8) Value {
	return ColorValue(Color{R: r, G: g, B: b, A: a})
}

func (v Value) validate() error {
	switch v.Kind {
	case KindNumber, KindAngle:
		if math.IsNaN(v.Number) || math.IsInf(v.Number, 0) {
			return fmt.Errorf("%s value must be finite, got %v", v.Kind, v.Number)
		}
	case KindBoolean, KindColor:
		// The fields are already fully constrained by their Go types.
	default:
		return fmt.Errorf("unknown parameter kind %v", v.Kind)
	}
	return nil
}
