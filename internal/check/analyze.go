package check

import (
	"fmt"
	"image"

	"proalpha/internal/imagex"
)

type FrameInput struct {
	Index              int
	Source             *image.NRGBA64
	Variant            *image.NRGBA64
	SourceAlphaMarked  bool
	VariantAlphaMarked bool
}

type AnalyzedFrame struct {
	Report   FrameReport
	Repaired *image.NRGBA64
}

func AnalyzeFrame(input FrameInput) AnalyzedFrame {
	variantModel, variantStats := imagex.ClassifySingleAlpha(input.Variant)
	report := FrameReport{
		Index: input.Index, Width: input.Variant.Rect.Dx(), Height: input.Variant.Rect.Dy(),
		VariantAlpha: variantStats, VariantModel: variantModel,
		Outputs: make(map[string]string),
	}

	var sourceStats imagex.AlphaStats
	sourceMeaningful := false
	if input.Source != nil {
		sourceStats = imagex.AnalyzeAlpha(input.Source)
		report.SourceAlpha = &sourceStats
		sourceMeaningful = sourceStats.NonOpaque > 0 &&
			float64(sourceStats.NonOpaque)/float64(sourceStats.Pixels) > 0.002

		metrics := MeanAbsError(input.Source, input.Variant)
		report.Metrics = &metrics
		models := CompareAlphaModel(input.Source, input.Variant)
		report.Models = &models

		if sourceMeaningful && variantStats.NonOpaque*4 < sourceStats.NonOpaque {
			report.AttachSourceAlpha = true
			report.WrongParameters = append(report.WrongParameters, WrongParam{
				Code: "pixel_alpha_channel_dropped", Severity: SeverityError, Frame: intPtr(input.Index),
				Parameter:    "RGBA.A",
				Expected:     fmt.Sprintf("source non-opaque pixels=%d", sourceStats.NonOpaque),
				Actual:       fmt.Sprintf("variant non-opaque pixels=%d", variantStats.NonOpaque),
				Message:      "decoded variant frame is opaque where the ProRes source has transparency",
				RepairAction: "copy aligned source alpha into repaired PNG; inspect RGB before using for production",
			})
		}

		if models.Winner == "premultiplied" {
			report.Unpremultiply = true
			report.WrongParameters = append(report.WrongParameters, WrongParam{
				Code: "pixel_premultiplied_rgb_misread_as_straight", Severity: SeverityError, Frame: intPtr(input.Index),
				Parameter:    "alpha_interpretation",
				Expected:     "straight RGB for PNG/normal players",
				Actual:       fmt.Sprintf("premultiplied model MAE=%.1f < straight MAE=%.1f", models.PremultipliedError, models.StraightRGBError),
				Message:      "RGB channels are already darkened by alpha; players treating them as straight RGBA show dark/black fringes",
				RepairAction: "unpremultiply RGB while preserving alpha in repaired frame",
			})
		} else if models.AlphaError > 2500 && !report.AttachSourceAlpha {
			report.WrongParameters = append(report.WrongParameters, WrongParam{
				Code: "pixel_alpha_mismatch", Severity: SeverityWarning, Frame: intPtr(input.Index),
				Parameter: "RGBA.A", Expected: "aligned source alpha",
				Actual:  fmt.Sprintf("mean absolute alpha error=%.1f/65535", models.AlphaError),
				Message: "alpha channels differ beyond ordinary compression noise",
			})
		}
	} else if variantModel == imagex.AlphaModelLikelyPremultiplied {
		report.Unpremultiply = true
		report.WrongParameters = append(report.WrongParameters, WrongParam{
			Code: "pixel_premultiplied_candidate_without_reference", Severity: SeverityWarning, Frame: intPtr(input.Index),
			Parameter: "alpha_interpretation", Expected: "straight RGBA",
			Actual:  "RGB <= A for most translucent pixels",
			Message: "single-file evidence suggests premultiplied RGB, but dark straight colors can look similar",
		})
	}

	repaired := imagex.CloneRGBA64(input.Variant)
	if report.AttachSourceAlpha && input.Source != nil {
		repaired = imagex.WithAlpha(repaired, input.Source)
	}
	if report.Unpremultiply {
		repaired = imagex.Unpremultiply(repaired)
	}

	report.RepairConfidence = confidence(report, input.Source != nil)
	return AnalyzedFrame{Report: report, Repaired: repaired}
}

func confidence(report FrameReport, hasSource bool) string {
	switch {
	case report.AttachSourceAlpha && report.Unpremultiply && hasSource:
		return "high: source alpha and channel model agree"
	case report.AttachSourceAlpha && hasSource:
		return "high: source alpha available"
	case report.Unpremultiply && report.Models != nil && report.Models.Winner == "premultiplied":
		return "medium: reference pixel model"
	case report.Unpremultiply:
		return "low: single-frame heuristic"
	default:
		return "high: no pixel repair required"
	}
}

func intPtr(value int) *int {
	return &value
}
