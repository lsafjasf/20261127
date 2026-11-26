package check

import (
	"fmt"

	"proalpha/internal/mediameta"
)

func NewFileReport(meta *mediameta.FileMetadata) FileReport {
	report := FileReport{Path: meta.Path}
	if meta.FFProbe != nil {
		stream := meta.FFProbe
		report.Codec = stream.CodecName
		report.Profile = stream.Profile
		report.PixelFormat = stream.PixelFormat
		report.Width = stream.Width
		report.Height = stream.Height
		report.ColorPrimaries = orDefault(stream.ColorPrimaries, movPrimaries(meta))
		report.ColorTransfer = orDefault(stream.ColorTransfer, movTransfer(meta))
		report.ColorMatrix = orDefault(stream.ColorSpace, movMatrix(meta))
		report.ColorRange = stream.ColorRange
	} else if meta.MOV != nil && meta.MOV.ColorBox != nil {
		box := meta.MOV.ColorBox
		report.ColorPrimaries = box.PrimariesName
		report.ColorTransfer = box.TransferName
		report.ColorMatrix = box.MatrixName
		if box.FullRangeFlag != nil {
			if *box.FullRangeFlag {
				report.ColorRange = "pc"
			} else {
				report.ColorRange = "tv"
			}
		}
	}
	if meta.MOV != nil && len(meta.MOV.ProResAlphaTypes) > 0 {
		alphaType := meta.MOV.ProResAlphaTypes[0]
		report.ProResAlphaType = &alphaType
	}
	report.MOVMetadata = meta.MOV
	return report
}

func ReferenceColor(meta *mediameta.FileMetadata) (primaries, transfer, matrix, rangeName string) {
	report := NewFileReport(meta)
	primaries = orDefault(report.ColorPrimaries, "bt709")
	transfer = orDefault(report.ColorTransfer, "bt709")
	matrix = orDefault(report.ColorMatrix, "bt709")
	rangeName = report.ColorRange
	return
}

func InternalMetadataWrongParams(meta *mediameta.FileMetadata) []WrongParam {
	if meta == nil {
		return nil
	}
	var issues []WrongParam
	path := meta.Path
	if meta.MOV != nil && meta.MOV.ProResAlphaMarked && meta.FFProbe != nil &&
		!mediameta.PixelFormatHasAlpha(meta.FFProbe.PixelFormat) {
		alphaType := 0
		if len(meta.MOV.ProResAlphaTypes) > 0 {
			alphaType = meta.MOV.ProResAlphaTypes[0]
		}
		fileReport := NewFileReport(meta)
		issues = append(issues, WrongParam{
			File: path, Code: "alpha_marker_pixfmt_conflict", Severity: SeverityError,
			Parameter: "pix_fmt", Expected: "yuva444p10le/yuva444p12le or rgba",
			Actual:  meta.FFProbe.PixelFormat,
			Message: fmt.Sprintf("ProRes frame header alpha_channel_type=%d says alpha exists, but ffprobe pixel format has no alpha", alphaType),
			FFmpegArguments: alphaArguments(
				orDefault(fileReport.ColorPrimaries, "bt709"),
				orDefault(fileReport.ColorTransfer, "bt709"),
				orDefault(fileReport.ColorMatrix, "bt709"),
				fileReport.ColorRange,
			),
			RepairAction: "re-encode to an alpha-capable proxy",
		})
	}
	if meta.MOV != nil && meta.MOV.ColorBox != nil && meta.FFProbe != nil {
		box := meta.MOV.ColorBox
		checks := []struct{ parameter, expected, actual string }{
			{"color_primaries", box.PrimariesName, meta.FFProbe.ColorPrimaries},
			{"color_trc", box.TransferName, meta.FFProbe.ColorTransfer},
			{"colorspace", box.MatrixName, meta.FFProbe.ColorSpace},
		}
		for _, item := range checks {
			if item.actual != "" && item.expected != "unknown" && item.actual != item.expected {
				issues = append(issues, WrongParam{
					File: path, Code: "colr_ffprobe_conflict", Severity: SeverityWarning,
					Parameter: item.parameter, Expected: item.expected, Actual: item.actual,
					Message: "MOV colr atom and decoded stream metadata disagree",
				})
			}
		}
	}
	return issues
}

func ComparisonMetadataWrongParams(source, candidate *mediameta.FileMetadata) []WrongParam {
	if source == nil || candidate == nil {
		return nil
	}
	var issues []WrongParam
	sourceReport := NewFileReport(source)
	candidateReport := NewFileReport(candidate)
	expectedPrimaries, expectedTransfer, expectedMatrix, expectedRange := ReferenceColor(source)
	fixArguments := alphaArguments(expectedPrimaries, expectedTransfer, expectedMatrix, expectedRange)

	sourceAlpha := sourceReport.ProResAlphaType != nil && *sourceReport.ProResAlphaType != 0
	sourceAlpha = sourceAlpha || mediameta.PixelFormatHasAlpha(sourceReport.PixelFormat)
	candidateAlpha := candidateReport.ProResAlphaType != nil && *candidateReport.ProResAlphaType != 0
	candidateAlpha = candidateAlpha || mediameta.PixelFormatHasAlpha(candidateReport.PixelFormat)

	if sourceAlpha && !candidateAlpha {
		actual := candidateReport.PixelFormat
		if actual == "" && candidateReport.ProResAlphaType != nil {
			actual = fmt.Sprintf("alpha_channel_type=%d", *candidateReport.ProResAlphaType)
		}
		issues = append(issues, WrongParam{
			File: candidate.Path, Code: "alpha_flag_missing_after_transcode", Severity: SeverityError,
			Parameter:       "alpha_channel_type/pix_fmt",
			Expected:        "alpha_channel_type=1|2 and yuva444p/rgba pixel format",
			Actual:          orDefault(actual, "no alpha marker"),
			Message:         "source carries an alpha marker, but the converted proxy cannot signal or store transparency",
			FFmpegArguments: fixArguments,
			RepairAction:    "rebuild proxy from source with ProRes 4444/QTRLE/PNG alpha; repaired PNG frames recover visible edges when source alpha is available",
		})
	}

	colorChecks := []struct{ parameter, expected, actual, flag string }{
		{"color_primaries", expectedPrimaries, candidateReport.ColorPrimaries, "-color_primaries"},
		{"color_trc", expectedTransfer, candidateReport.ColorTransfer, "-color_trc"},
		{"colorspace", expectedMatrix, candidateReport.ColorMatrix, "-colorspace"},
	}
	for _, item := range colorChecks {
		if item.actual == "" {
			issues = append(issues, WrongParam{
				File: candidate.Path, Code: "color_descriptor_missing", Severity: SeverityWarning,
				Parameter: item.parameter, Expected: item.expected, Actual: "unspecified",
				Message:         "missing color descriptor makes players choose different RGB/YUV and range heuristics",
				FFmpegArguments: []string{item.flag, item.expected},
				RepairAction:    "set the same explicit color descriptor as the ProRes source",
			})
		} else if item.actual != item.expected {
			issues = append(issues, WrongParam{
				File: candidate.Path, Code: "color_descriptor_mismatch", Severity: SeverityError,
				Parameter: item.parameter, Expected: item.expected, Actual: item.actual,
				Message:         "converted proxy changes the color description; this can shift channels and fringe interpretation",
				FFmpegArguments: []string{item.flag, item.expected},
				RepairAction:    "re-encode or rewrite metadata with the source color tags",
			})
		}
	}
	appendColorRangeIssue(&issues, candidate.Path, expectedRange, candidateReport.ColorRange)
	return issues
}

func appendColorRangeIssue(issues *[]WrongParam, path, sourceRange, candidateRange string) {
	switch {
	case sourceRange != "" && candidateRange == "":
		*issues = append(*issues, WrongParam{
			File: path, Code: "color_range_missing", Severity: SeverityWarning,
			Parameter: "color_range", Expected: sourceRange, Actual: "unspecified",
			Message:         "source declares range but converted proxy omits it",
			FFmpegArguments: []string{"-color_range", sourceRange},
			RepairAction:    "preserve the source color range",
		})
	case sourceRange != "" && candidateRange != sourceRange:
		*issues = append(*issues, WrongParam{
			File: path, Code: "color_range_mismatch", Severity: SeverityError,
			Parameter: "color_range", Expected: sourceRange, Actual: candidateRange,
			Message:         "explicit color range differs from the ProRes source",
			FFmpegArguments: []string{"-color_range", sourceRange},
			RepairAction:    "re-encode or rewrite metadata with the source range",
		})
	case sourceRange == "" && candidateRange == "":
		*issues = append(*issues, WrongParam{
			File: path, Code: "color_range_unspecified_both", Severity: SeverityInfo,
			Parameter: "color_range", Expected: "explicit tv or pc from delivery contract", Actual: "unspecified",
			Message:         "neither file declares range; this is not a source/variant conflict, but players may still choose different defaults",
			FFmpegArguments: []string{"-color_range", "tv"},
			RepairAction:    "set tv for conventional limited-range SDR YUV, or pc only if the master is full-range",
		})
	case sourceRange == "" && candidateRange != "tv":
		*issues = append(*issues, WrongParam{
			File: path, Code: "color_range_needs_confirmation", Severity: SeverityWarning,
			Parameter: "color_range", Expected: "tv unless master is full-range pc", Actual: candidateRange,
			Message:         "source range is unspecified and variant uses a non-default explicit range",
			FFmpegArguments: []string{"-color_range", "tv"},
			RepairAction:    "confirm the mastering range before changing pixels or tags",
		})
	}
}

func alphaPreservingArguments() []string {
	return alphaArguments("bt709", "bt709", "bt709", "tv")
}

func alphaArguments(primaries, transfer, matrix, rangeName string) []string {
	arguments := []string{
		"-c:v", "prores_ks", "-profile:v", "4444",
		"-pix_fmt", "yuva444p10le", "-alpha_bits", "16",
		"-color_primaries", primaries, "-color_trc", transfer,
		"-colorspace", matrix,
	}
	if rangeName != "" {
		arguments = append(arguments, "-color_range", rangeName)
	}
	return arguments
}

func orDefault(value, fallback string) string {
	if value != "" && value != "unknown" {
		return value
	}
	return fallback
}

func movPrimaries(meta *mediameta.FileMetadata) string {
	if meta.MOV != nil && meta.MOV.ColorBox != nil {
		return meta.MOV.ColorBox.PrimariesName
	}
	return ""
}

func movTransfer(meta *mediameta.FileMetadata) string {
	if meta.MOV != nil && meta.MOV.ColorBox != nil {
		return meta.MOV.ColorBox.TransferName
	}
	return ""
}

func movMatrix(meta *mediameta.FileMetadata) string {
	if meta.MOV != nil && meta.MOV.ColorBox != nil {
		return meta.MOV.ColorBox.MatrixName
	}
	return ""
}
