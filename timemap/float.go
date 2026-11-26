package timemap

import "math"

// floatQuantize mirrors the integer-casting / math.Round calls found in real
// media pipelines and reports whether the float value was already integral.
func floatQuantize(f float64, rule RoundingRule) (idx int64, isInt bool) {
	isInt = f == math.Trunc(f) && !math.IsInf(f, 0)
	switch rule {
	case RoundFloor:
		idx = int64(math.Floor(f))
	case RoundCeil:
		idx = int64(math.Ceil(f))
	case RoundNearest:
		idx = int64(math.Round(f))
	default:
		panic("unreachable")
	}
	return idx, isInt
}
