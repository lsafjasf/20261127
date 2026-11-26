package manifest

import (
	"fmt"
	"strings"
)

type resolvedColorRequirement struct {
	Primaries               string
	AllowedTransfers        []string
	Matrix                  string
	Range                   string
	BitDepth                int
	RequireMasteringDisplay *bool
	RequireContentLight     *bool
}

func resolveColorRequirement(role string, config ColorConfig) resolvedColorRequirement {
	resolved := resolvedColorRequirement{
		Primaries: config.Primaries,
		Matrix:    config.Matrix,
		Range:     config.Range,
		BitDepth:  config.BitDepth,
	}
	if config.Transfer != "" {
		resolved.AllowedTransfers = []string{config.Transfer}
	} else {
		resolved.AllowedTransfers = append([]string(nil), config.AllowedTransfers...)
	}
	resolved.RequireMasteringDisplay = config.RequireMasteringDisplay
	resolved.RequireContentLight = config.RequireContentLight

	if strings.EqualFold(role, "HDR") {
		resolved.Primaries = firstNonEmpty(resolved.Primaries, "bt2020")
		resolved.Matrix = firstNonEmpty(resolved.Matrix, "bt2020nc")
		resolved.Range = firstNonEmpty(resolved.Range, "tv")
		if resolved.BitDepth == 0 {
			resolved.BitDepth = 10
		}
		if len(resolved.AllowedTransfers) == 0 {
			resolved.AllowedTransfers = []string{"smpte2084", "arib-std-b67"}
		}
	} else {
		resolved.Primaries = firstNonEmpty(resolved.Primaries, "bt709")
		resolved.Matrix = firstNonEmpty(resolved.Matrix, "bt709")
		resolved.Range = firstNonEmpty(resolved.Range, "tv")
		if len(resolved.AllowedTransfers) == 0 {
			resolved.AllowedTransfers = []string{"bt709"}
		}
		if resolved.BitDepth == 0 {
			resolved.BitDepth = 8
		}
		if resolved.RequireMasteringDisplay == nil {
			value := false
			resolved.RequireMasteringDisplay = &value
		}
		if resolved.RequireContentLight == nil {
			value := false
			resolved.RequireContentLight = &value
		}
	}
	return resolved
}

func (a *assessor) buildColor(role string, config ColorConfig, probe ProbeData, probeErr error) ColorReport {
	requirement := resolveColorRequirement(role, config)
	actual := colorActualFromProbe(probe)
	report := ColorReport{
		Status:   StatusIncomplete,
		Standard: colorStandard(actual),
		Actual:   actual,
		Checks:   []Check{},
	}
	if probeErr != nil {
		report.Checks = append(report.Checks, Check{
			ID:       "color.video_probe",
			Category: "color",
			Required: true,
			Pass:     false,
			Expected: "readable video stream and color metadata",
			Actual:   "unavailable",
			Detail:   "color metadata cannot be checked because probing failed",
		})
		return report
	}

	report.Checks = append(report.Checks,
		exactStringCheck("color.primaries", requirement.Primaries, actual.Primaries, "color primaries"),
		allowedStringCheck("color.transfer", requirement.AllowedTransfers, actual.Transfer, "transfer characteristics"),
		exactStringCheck("color.matrix", requirement.Matrix, actual.Matrix, "color matrix"),
		exactStringCheck("color.range", requirement.Range, actual.Range, "color range"),
	)

	depthCheck := Check{
		ID:       "color.bit_depth",
		Category: "color",
		Required: true,
		Expected: requirement.BitDepth,
		Actual:   actual.BitDepth,
		Pass:     actual.BitDepth == requirement.BitDepth,
	}
	if !depthCheck.Pass {
		depthCheck.Detail = fmt.Sprintf("expected %d-bit video, got %s", requirement.BitDepth, bitDepthText(actual.BitDepth))
	}
	report.Checks = append(report.Checks, depthCheck)

	requireMastering := requirement.RequireMasteringDisplay
	if requireMastering == nil {
		required := !strings.EqualFold(actual.Transfer, "arib-std-b67")
		requireMastering = &required
	}
	requireLight := requirement.RequireContentLight
	if requireLight == nil {
		required := !strings.EqualFold(actual.Transfer, "arib-std-b67")
		requireLight = &required
	}
	if *requireMastering {
		check := Check{
			ID:       "color.mastering_display",
			Category: "color",
			Required: true,
			Expected: "present",
			Actual:   boolText(actual.MasteringDisplay),
			Pass:     actual.MasteringDisplay,
		}
		if !check.Pass {
			check.Detail = "HDR mastering display metadata is missing"
		}
		report.Checks = append(report.Checks, check)
	}
	if *requireLight {
		check := Check{
			ID:       "color.content_light",
			Category: "color",
			Required: true,
			Expected: "present",
			Actual:   boolText(actual.ContentLight),
			Pass:     actual.ContentLight,
		}
		if !check.Pass {
			check.Detail = "HDR content light level metadata is missing"
		}
		report.Checks = append(report.Checks, check)
	}
	report.Status = statusFromChecks(report.Checks)
	return report
}

func colorActualFromProbe(probe ProbeData) ColorActual {
	stream, ok := probe.FirstVideo()
	if !ok {
		return ColorActual{}
	}
	actual := ColorActual{
		Primaries:   stream.ColorPrimaries,
		Transfer:    stream.ColorTransfer,
		Matrix:      stream.ColorSpace,
		Range:       stream.ColorRange,
		PixelFormat: stream.PixelFormat,
		BitDepth:    PixelBitDepth(stream),
	}
	var frameSideData []ProbeSideData
	if len(probe.Frames) > 0 {
		frame := probe.Frames[0]
		actual.Primaries = firstNonEmpty(actual.Primaries, frame.ColorPrimaries)
		actual.Transfer = firstNonEmpty(actual.Transfer, frame.ColorTransfer)
		actual.Matrix = firstNonEmpty(actual.Matrix, frame.ColorSpace)
		actual.Range = firstNonEmpty(actual.Range, frame.ColorRange)
		actual.PixelFormat = firstNonEmpty(actual.PixelFormat, frame.PixelFormat)
		frameSideData = frame.SideDataList
	}
	sideData := stream.AllSideData(frameSideData)
	actual.MasteringDisplay = HasSideData(sideData, "mastering display")
	actual.ContentLight = HasSideData(sideData, "content light level")
	if mastering, found := FindSideData(sideData, "mastering display"); found {
		actual.MasteringDisplayDetails = fmt.Sprintf(
			"R=%s,%s G=%s,%s B=%s,%s WP=%s,%s L=%s,%s",
			mastering.RedX, mastering.RedY,
			mastering.GreenX, mastering.GreenY,
			mastering.BlueX, mastering.BlueY,
			mastering.WhitePointX, mastering.WhitePointY,
			mastering.MaxLuminance, mastering.MinLuminance,
		)
	}
	if light, found := FindSideData(sideData, "content light level"); found {
		actual.MaxCLL = light.MaxContent
		actual.MaxFALL = light.MaxAverage
	}
	return actual
}

func colorStandard(actual ColorActual) string {
	switch strings.ToLower(actual.Transfer) {
	case "smpte2084":
		if strings.EqualFold(actual.Primaries, "bt2020") {
			return "HDR10/PQ"
		}
		return "PQ"
	case "arib-std-b67":
		return "HLG"
	case "bt709":
		if strings.EqualFold(actual.Primaries, "bt709") {
			return "SDR/BT.709"
		}
		return "SDR"
	default:
		return "unknown"
	}
}

func exactStringCheck(id, expected, actual, label string) Check {
	check := Check{
		ID:       id,
		Category: "color",
		Required: true,
		Expected: expected,
		Actual:   nonEmptyValue(actual),
		Pass:     strings.EqualFold(expected, actual),
	}
	if !check.Pass {
		check.Detail = fmt.Sprintf("expected %s %q, got %s", label, expected, nonEmptyValue(actual))
	}
	return check
}

func allowedStringCheck(id string, expected []string, actual, label string) Check {
	check := Check{
		ID:       id,
		Category: "color",
		Required: true,
		Expected: expected,
		Actual:   nonEmptyValue(actual),
	}
	for _, allowed := range expected {
		if strings.EqualFold(allowed, actual) {
			check.Pass = true
			break
		}
	}
	if !check.Pass {
		check.Detail = fmt.Sprintf("expected %s in %v, got %s", label, expected, nonEmptyValue(actual))
	}
	return check
}

func nonEmptyValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "missing"
	}
	return value
}

func boolText(value bool) string {
	if value {
		return "present"
	}
	return "missing"
}

func bitDepthText(depth int) string {
	if depth == 0 {
		return "unknown depth"
	}
	return fmt.Sprintf("%d-bit", depth)
}
