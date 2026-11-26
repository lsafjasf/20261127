package rangecheck

import "fmt"

func classifyRegion(r *Region, frame Frame, settings Settings) {
	bounds := settings.LumaBounds()
	frameArea := frame.Width * frame.Height
	areaRatio := float64(r.Rect.Area()) / float64(frameArea)
	widthHeight := float64(r.Rect.Width) / float64(max(1, r.Rect.Height))

	depth := 0
	extreme := false
	if r.Kind == KindSuperBlack {
		depth = bounds.Black - r.MinCode
		extreme = r.MinCode <= 1
	} else {
		depth = r.MaxCode - bounds.White
		extreme = r.MaxCode >= 254
	}

	r.Reasons = append(r.Reasons,
		fmt.Sprintf("code_depth_beyond_legal=%d", depth),
		fmt.Sprintf("bbox=%.2f%%frame fill=%.3f contrast=%.1f", areaRatio*100, r.FillRatio, r.LocalContrast),
	)

	switch {
	case r.EdgeLike && depth <= 4 && r.LocalContrast >= 24:
		r.Source = SourceCodecEdge
		r.Verdict = VerdictEncodingError
		r.Confidence = clampConfidence(0.72 + float64(4-depth)*0.03)
		r.Reasons = append(r.Reasons, "thin_high_contrast_halo_and_small_overshoot")

	case areaRatio >= 0.25 && r.FillRatio >= 0.78 && meanOvershoot(r, bounds) <= 3:
		r.Source = SourceLevelShift
		r.Verdict = VerdictEncodingError
		r.Confidence = 0.64
		r.Reasons = append(r.Reasons, "large_flat_region_with_only_one_to_few_code_level_offset")

	case r.Kind == KindSuperBlack && textGeometry(r, widthHeight):
		if r.BottomBand {
			r.Source = SourceBurnedSubtitle
			r.Verdict = VerdictEncodingError
			r.Confidence = 0.78
			r.Confidence += 0.08
			r.Reasons = append(r.Reasons, "located_in_lower_title_safe_area")
			if r.ComponentCount >= 3 {
				r.Confidence += 0.05
				r.Reasons = append(r.Reasons, "multiple_glyph_like_components_grouped_on_one_line")
			}
			if extreme {
				r.Confidence += 0.04
				r.Reasons = append(r.Reasons, "anchored_at_code_0_consistent_with_RGB_black_written_directly_to_Y")
			}
			r.Confidence = clampConfidence(r.Confidence)
		} else {
			r.Source = SourceGraphicText
			r.Verdict = VerdictEncodingError
			r.Confidence = 0.61
			r.Reasons = append(r.Reasons, "horizontal_glyph_cluster_outside_lower_subtitle_band")
			if r.ComponentCount >= 5 {
				r.Confidence += 0.05
				r.Reasons = append(r.Reasons, "multiple_glyph_like_components_grouped_on_one_line")
			}
			if extreme {
				r.Confidence += 0.03
				r.Reasons = append(r.Reasons, "code_0_anchor_suggests_direct_full_range_graphics")
			}
			r.Confidence = clampConfidence(r.Confidence)
		}

	case areaRatio >= 0.08 && r.FillRatio >= 0.30 && !r.EdgeLike:
		r.Source = SourceStylizedGrade
		r.Verdict = VerdictIntentionalStyle
		r.Confidence = 0.58
		r.Reasons = append(r.Reasons, "large_coherent_extreme_tone_region_not_text_or_thin_ringing")
		if extreme {
			r.Reasons = append(r.Reasons, "hard_extreme_anchor_can_be_a_crushed_grade_but_is_not_limited_range_legal")
		}

	case r.Kind == KindSuperWhite && highlightGeometry(r):
		r.Source = SourceHighlight
		r.Confidence = 0.62
		r.Reasons = append(r.Reasons, "bright_compact_or_scene_locked_region_adjacent_to_strong_highlights")
		if r.ContextMean >= 175 {
			r.Confidence += 0.08
			r.Reasons = append(r.Reasons, "surrounded_by_near_legal_highlight_values")
		}
		if extreme {
			r.Confidence += 0.04
			r.Reasons = append(r.Reasons, "code_255_anchor_suggests_unconverted_full_range_or_hard_clip")
		}
		r.Verdict = VerdictAmbiguous

	default:
		r.Source = SourceUnknown
		r.Verdict = VerdictAmbiguous
		r.Confidence = 0.35
		r.Reasons = append(r.Reasons, "geometry_is_not_diagnostic_from_one_frame")
	}

	if r.Confidence < 0.55 && r.Verdict != VerdictAmbiguous {
		r.Verdict = VerdictAmbiguous
	}
}

func textGeometry(r *Region, aspect float64) bool {
	if r.Rect.Height < 8 {
		return false
	}
	horizontalLine := aspect >= 2.2 && aspect <= 40
	glyphFill := r.FillRatio >= 0.08 && r.FillRatio <= 0.82
	return (r.BottomBand && horizontalLine && glyphFill) ||
		(horizontalLine && glyphFill && r.ComponentCount >= 5)
}

func highlightGeometry(r *Region) bool {
	if r.Rect.Width < 3 || r.Rect.Height < 3 {
		return false
	}
	return r.ContextMean >= 90 || r.FillRatio >= 0.12
}

func meanOvershoot(r *Region, bounds LevelBounds) float64 {
	if r.Kind == KindSuperBlack {
		return float64(bounds.Black) - r.MeanCode
	}
	return r.MeanCode - float64(bounds.White)
}

func clampConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 0.96 {
		return 0.96
	}
	return v
}
