package hdrswitch

import (
	"fmt"
	"math"
)

func cloneRationalPtr(value *Rational) *Rational {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func mergeMetadata(base ColorMetadata, patch *ColorMetadata) ColorMetadata {
	if patch == nil {
		return base
	}
	out := base

	if patch.Transfer != "" {
		out.Transfer = patch.Transfer
	}
	if patch.TransferCode != 0 {
		out.TransferCode = patch.TransferCode
	}
	if patch.Primaries != "" {
		out.Primaries = patch.Primaries
	}
	if patch.PrimariesCode != 0 {
		out.PrimariesCode = patch.PrimariesCode
	}
	if patch.Matrix != "" {
		out.Matrix = patch.Matrix
	}
	if patch.MatrixCode != 0 {
		out.MatrixCode = patch.MatrixCode
	}
	if patch.FullRange != nil {
		out.FullRange = boolPtr(*patch.FullRange)
	}
	if patch.BitDepth != 0 {
		out.BitDepth = patch.BitDepth
	}
	if patch.NominalPeakNits != nil {
		out.NominalPeakNits = cloneRationalPtr(patch.NominalPeakNits)
	}
	if patch.MaxCLL != nil {
		out.MaxCLL = cloneRationalPtr(patch.MaxCLL)
	}
	if patch.MaxFALL != nil {
		out.MaxFALL = cloneRationalPtr(patch.MaxFALL)
	}
	if patch.Mastering != nil {
		if out.Mastering == nil {
			out.Mastering = &MasteringVolume{}
		}
		merged := *out.Mastering
		patchVolume := patch.Mastering
		if patchVolume.Primaries != "" {
			merged.Primaries = patchVolume.Primaries
		}
		if len(patchVolume.PrimariesXY) > 0 {
			merged.PrimariesXY = append([]Point(nil), patchVolume.PrimariesXY...)
		}
		if patchVolume.WhitePoint != nil {
			white := *patchVolume.WhitePoint
			merged.WhitePoint = &white
		}
		if patchVolume.MaxLuminanceNits != nil {
			merged.MaxLuminanceNits = cloneRationalPtr(patchVolume.MaxLuminanceNits)
		}
		if patchVolume.MinLuminanceNits != nil {
			merged.MinLuminanceNits = cloneRationalPtr(patchVolume.MinLuminanceNits)
		}
		out.Mastering = &merged
	}
	return out
}

func rationalPtrEqual(a, b *Rational) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Cmp(*b) == 0
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func diffMetadata(before, after ColorMetadata) []string {
	changes := []string{}

	beforeTransfer, beforeCode, beforeErr := normalizeTransfer(&before)
	afterTransfer, afterCode, afterErr := normalizeTransfer(&after)
	if beforeErr == nil && afterErr == nil && beforeTransfer != afterTransfer {
		changes = append(changes, "transfer:"+beforeTransfer+"->"+afterTransfer)
	} else if before.Transfer != after.Transfer || before.TransferCode != after.TransferCode {
		changes = append(changes, "transfer")
	}
	if beforeCode != afterCode && beforeErr == nil && afterErr == nil {
		// Canonical names already capture the change; code remains in report.
	}

	beforeGamut, err1 := normalizePrimaries(&before)
	afterGamut, err2 := normalizePrimaries(&after)
	if err1 == nil && err2 == nil && beforeGamut.Name != afterGamut.Name {
		changes = append(changes, "primaries:"+beforeGamut.Name+"->"+afterGamut.Name)
	} else if before.Primaries != after.Primaries || before.PrimariesCode != after.PrimariesCode {
		changes = append(changes, "primaries")
	}
	if before.Matrix != after.Matrix || before.MatrixCode != after.MatrixCode {
		changes = append(changes, "matrix")
	}
	if !boolPtrEqual(before.FullRange, after.FullRange) {
		changes = append(changes, "full_range")
	}
	if before.BitDepth != after.BitDepth {
		changes = append(changes, "bit_depth")
	}
	if !rationalPtrEqual(before.NominalPeakNits, after.NominalPeakNits) {
		changes = append(changes, "nominal_peak")
	}
	if !rationalPtrEqual(before.MaxCLL, after.MaxCLL) {
		changes = append(changes, "max_cll")
	}
	if !rationalPtrEqual(before.MaxFALL, after.MaxFALL) {
		changes = append(changes, "max_fall")
	}

	beforeVolume := before.Mastering
	afterVolume := after.Mastering
	if (beforeVolume == nil) != (afterVolume == nil) {
		changes = append(changes, "mastering_volume")
	} else if beforeVolume != nil && afterVolume != nil {
		if beforeVolume.Primaries != afterVolume.Primaries || len(beforeVolume.PrimariesXY) != len(afterVolume.PrimariesXY) {
			changes = append(changes, "mastering_primaries")
		} else {
			for i := range beforeVolume.PrimariesXY {
				if math.Abs(beforeVolume.PrimariesXY[i].X-afterVolume.PrimariesXY[i].X) > 1e-9 ||
					math.Abs(beforeVolume.PrimariesXY[i].Y-afterVolume.PrimariesXY[i].Y) > 1e-9 {
					changes = append(changes, "mastering_primaries")
					break
				}
			}
		}
		if !pointPtrEqual(beforeVolume.WhitePoint, afterVolume.WhitePoint) {
			changes = append(changes, "mastering_white")
		}
		if !rationalPtrEqual(beforeVolume.MaxLuminanceNits, afterVolume.MaxLuminanceNits) {
			changes = append(changes, "mastering_max_luminance")
		}
		if !rationalPtrEqual(beforeVolume.MinLuminanceNits, afterVolume.MinLuminanceNits) {
			changes = append(changes, "mastering_min_luminance")
		}
	}
	return uniqueStrings(changes)
}

func pointPtrEqual(a, b *Point) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return math.Abs(a.X-b.X) <= 1e-9 && math.Abs(a.Y-b.Y) <= 1e-9
}

func stickyMismatchFields(sticky, current ColorMetadata) []string {
	return diffMetadata(sticky, current)
}

func validateColorMetadata(at int64, meta ColorMetadata) []Finding {
	findings := []Finding{}
	add := func(code, severity, format string, args ...any) {
		au := at
		findings = append(findings, Finding{
			Code: code, Severity: severity, AUIndex: &au,
			Message: fmt.Sprintf(format, args...),
		})
	}

	if _, _, err := normalizeTransfer(&meta); err != nil {
		add("invalid_transfer_metadata", SeverityError, "AU %d: %v", at, err)
	}
	if _, err := normalizePrimaries(&meta); err != nil {
		add("invalid_gamut_metadata", SeverityError, "AU %d: %v", at, err)
	}

	for _, candidate := range []struct {
		name  string
		value *Rational
	}{
		{"nominal_peak_nits", meta.NominalPeakNits},
		{"max_cll", meta.MaxCLL},
		{"max_fall", meta.MaxFALL},
	} {
		if candidate.value != nil && candidate.value.Num <= 0 {
			add("invalid_peak_value", SeverityError, "AU %d: %s must be positive", at, candidate.name)
		}
	}

	if meta.MaxCLL != nil && meta.MaxCLL.Num > 65535 {
		add("light_level_field_overflow", SeverityError, "AU %d: MaxCLL=%s nits does not fit uint16 CTA-861.3 field", at, meta.MaxCLL.String())
	}
	if meta.MaxFALL != nil && meta.MaxFALL.Num > 65535 {
		add("light_level_field_overflow", SeverityError, "AU %d: MaxFALL=%s nits does not fit uint16 CTA-861.3 field", at, meta.MaxFALL.String())
	}
	if pointerLess(meta.MaxCLL, meta.MaxFALL) {
		add("fall_greater_than_cll", SeverityWarning, "AU %d: MaxFALL=%s exceeds MaxCLL=%s", at, meta.MaxFALL.String(), meta.MaxCLL.String())
	}

	if meta.Mastering != nil {
		volume := meta.Mastering
		if len(volume.PrimariesXY) != 0 && len(volume.PrimariesXY) != 3 {
			add("invalid_mastering_primaries", SeverityError, "AU %d: mastering_display_colour_volume requires exactly three primary xy pairs", at)
		}
		for i, point := range volume.PrimariesXY {
			if !validCIEPoint(point) {
				add("invalid_cie_coordinate", SeverityError, "AU %d: primary %d xy=(%g,%g) is outside [0,1]", at, i, point.X, point.Y)
			}
		}
		if len(volume.PrimariesXY) == 3 {
			a := volume.PrimariesXY[0]
			b := volume.PrimariesXY[1]
			c := volume.PrimariesXY[2]
			area2 := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
			if math.Abs(area2) < 1e-9 {
				add("degenerate_gamut_triangle", SeverityError, "AU %d: mastering display primaries form a degenerate triangle", at)
			}
		}
		if volume.WhitePoint != nil && !validCIEPoint(*volume.WhitePoint) {
			add("invalid_cie_coordinate", SeverityError, "AU %d: white point xy=(%g,%g) is outside [0,1]", at, volume.WhitePoint.X, volume.WhitePoint.Y)
		}
		if volume.MaxLuminanceNits != nil {
			max := volume.MaxLuminanceNits
			if max.Num <= 0 {
				add("invalid_mastering_luminance", SeverityError, "AU %d: max mastering luminance must be positive", at)
			} else if max.Cmp(MustRational(10000, 1)) > 0 {
				add("mastering_luminance_above_st2086", SeverityWarning, "AU %d: max mastering luminance %s nits is outside ST 2086 nominal [5,10000]", at, max.String())
			} else if max.Cmp(MustRational(5, 1)) < 0 {
				add("mastering_luminance_below_st2086", SeverityWarning, "AU %d: max mastering luminance %s nits is below ST 2086 nominal [5,10000]", at, max.String())
			}
		}
		if volume.MinLuminanceNits != nil && volume.MinLuminanceNits.Num != 0 {
			min := volume.MinLuminanceNits
			if min.Num < 0 {
				add("invalid_mastering_luminance", SeverityError, "AU %d: min mastering luminance must be non-negative", at)
			} else if min.Cmp(MustRational(5, 1)) > 0 || min.Cmp(MustRational(1, 10000)) < 0 {
				add("mastering_luminance_outside_st2086", SeverityWarning, "AU %d: min mastering luminance %s nits is outside nominal [0.0001,5]", at, min.String())
			}
		}
		if meta.MaxCLL != nil && volume.MaxLuminanceNits != nil && meta.MaxCLL.Cmp(*volume.MaxLuminanceNits) > 0 {
			add("cll_exceeds_mastering_volume", SeverityWarning, "AU %d: MaxCLL=%s exceeds max mastering luminance=%s", at, meta.MaxCLL.String(), volume.MaxLuminanceNits.String())
		}
	}
	return findings
}

func validCIEPoint(point Point) bool {
	return point.X >= 0 && point.X <= 1 && point.Y >= 0 && point.Y <= 1
}
