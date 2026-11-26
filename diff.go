package hdrframe

import "math"

func diffVersionParams(expected, applied *VersionRef) []ParamDiff {
	if expected == nil || applied == nil {
		return nil
	}
	var diffs []ParamDiff
	add := func(field, label string, want, got float64, effect string) {
		if math.Abs(want-got) < 0.001 && (want != 0 || got != 0) {
			return
		}
		if want == 0 && got == 0 {
			return
		}
		diffs = append(diffs, ParamDiff{
			Field: field, Label: label, Expected: want, Applied: got,
			Delta: got - want, Effect: effectForDirection(field, want, got, effect),
		})
	}

	wantPeak := expected.Highlight.TargetPeakNits
	gotPeak := applied.Highlight.TargetPeakNits
	if wantPeak == 0 {
		wantPeak = expected.Highlight.PeakNits
	}
	if gotPeak == 0 {
		gotPeak = applied.Highlight.PeakNits
	}
	add("highlight.target_peak_nits", "局部高光目标峰值(nits)", wantPeak, gotPeak, "峰值低于预期时局部高光更暗，高于预期时可能过亮")
	add("highlight.gain", "高光增益", expected.Highlight.Gain, applied.Highlight.Gain, "高光增益不一致会改变局部亮度层次")
	add("color.hue_shift_degrees", "色相旋转(度)", expected.Color.HueShiftDegrees, applied.Color.HueShiftDegrees, "色相旋转量不一致，高光色相偏离创作意图")
	add("color.saturation", "饱和度", expected.Color.Saturation, applied.Color.Saturation, "饱和度不一致会让霓虹/彩色高光浓淡异常")
	return diffs
}

func effectForDirection(field string, want, got float64, fallback string) string {
	if math.Abs(want-got) < 0.001 {
		return ""
	}
	switch field {
	case "highlight.target_peak_nits", "highlight.gain":
		if got < want {
			return "实际值低于应有版本：局部高光亮度/层次未按预期提升"
		}
		return "实际值高于应有版本：局部高光可能过冲或过曝"
	case "color.hue_shift_degrees":
		if got > want {
			return "实际色相旋转偏正：高光颜色会向另一色相方向偏移"
		}
		return "实际色相旋转偏负：高光颜色会向相反色相方向偏移"
	default:
		return fallback
	}
}
