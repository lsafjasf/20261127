package effectinterp

import (
	"fmt"
	"math"
)

// InterpolateNumber linearly interpolates from to to according to progress.
// Progress is clamped to [0, 1]; NaN is treated as zero.
func InterpolateNumber(from, to, progress float64) float64 {
	t := clampProgress(progress)
	if t == 0 {
		return from
	}
	if t == 1 {
		return to
	}
	return from + (to-from)*t
}

// InterpolateAngle interpolates along the shortest circular path.
//
// Both endpoints and the result are normalized to [0, 360). Values that differ
// by a full turn are therefore equivalent: 0 and 360 are the same angle, and
// 350 to 10 travels forward through 0 rather than backward through 180. When
// the two directions are equally short (a 180-degree difference), the
// increasing direction is used. Use InterpolateNumber when multiple full
// rotations must be preserved.
func InterpolateAngle(from, to, progress float64) float64 {
	t := clampProgress(progress)
	start := NormalizeAngle(from)
	end := NormalizeAngle(to)
	if t == 0 {
		return start
	}
	if t == 1 {
		return end
	}

	delta := end - start
	if delta > 180 {
		delta -= 360
	} else if delta <= -180 {
		delta += 360
	}
	return NormalizeAngle(start + delta*t)
}

// InterpolateBoolean holds the left value until the interpolation reaches its
// end. In other words, booleans are stepped, not blended.
func InterpolateBoolean(from, to bool, progress float64) bool {
	if clampProgress(progress) == 1 {
		return to
	}
	return from
}

// InterpolateColor interpolates each RGBA component independently. Components
// are rounded to the nearest integer, with exact halves rounded up. Alpha is
// interpolated with the same rule as the color channels.
func InterpolateColor(from, to Color, progress float64) Color {
	t := clampProgress(progress)
	return Color{
		R: interpolateByte(from.R, to.R, t),
		G: interpolateByte(from.G, to.G, t),
		B: interpolateByte(from.B, to.B, t),
		A: interpolateByte(from.A, to.A, t),
	}
}

// Interpolate dispatches to the interpolator selected by kind. Both values must
// have the supplied kind and must be valid.
func Interpolate(kind Kind, from, to Value, progress float64) (Value, error) {
	if !kind.valid() {
		return Value{}, fmt.Errorf("unknown parameter kind %v", kind)
	}
	if from.Kind != kind || to.Kind != kind {
		return Value{}, fmt.Errorf(
			"cannot interpolate %v and %v as %v",
			from.Kind, to.Kind, kind,
		)
	}
	if err := from.validate(); err != nil {
		return Value{}, fmt.Errorf("from value: %w", err)
	}
	if err := to.validate(); err != nil {
		return Value{}, fmt.Errorf("to value: %w", err)
	}

	switch kind {
	case KindNumber:
		return NumberValue(InterpolateNumber(from.Number, to.Number, progress)), nil
	case KindAngle:
		return AngleValue(InterpolateAngle(from.Number, to.Number, progress)), nil
	case KindBoolean:
		return BooleanValue(InterpolateBoolean(from.Boolean, to.Boolean, progress)), nil
	case KindColor:
		return ColorValue(InterpolateColor(from.Color, to.Color, progress)), nil
	default:
		panic("unreachable")
	}
}

// NormalizeAngle maps an angle to the equivalent value in [0, 360).
func NormalizeAngle(degrees float64) float64 {
	angle := math.Mod(degrees, 360)
	if angle < 0 {
		angle += 360
	}
	if angle == 0 {
		return 0
	}
	return angle
}

func clampProgress(progress float64) float64 {
	switch {
	case math.IsNaN(progress), progress <= 0:
		return 0
	case progress >= 1:
		return 1
	default:
		return progress
	}
}

func interpolateByte(from, to uint8, progress float64) uint8 {
	if progress == 0 {
		return from
	}
	if progress == 1 {
		return to
	}
	v := float64(from) + (float64(to)-float64(from))*progress
	return uint8(math.Round(v))
}
