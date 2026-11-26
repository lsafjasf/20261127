package main

import (
	"fmt"
	"io"
	"strings"

	"streamcheck/internal/check"
)

func renderText(w io.Writer, report *check.Report) {
	fmt.Fprintf(w, "已比较 %d 个帧的容器、码流 CRC、解码与像素证据。\n", report.FrameCount)

	if report.Earliest != nil {
		fmt.Fprintf(w, "最早硬异常（解码/因果序）：帧 %s（decode_order=%d），%s / %s。\n",
			report.Earliest.FrameKey, report.Earliest.DecodeOrder,
			layerName(report.Earliest.Layer), report.Earliest.Check)
	} else {
		fmt.Fprintln(w, "未发现 status=fail 的硬异常；仍需查看 warn/unknown 与可信像素证据。")
	}

	if report.EarliestDisplay != nil && report.Earliest != nil &&
		(report.EarliestDisplay.FrameKey != report.Earliest.FrameKey ||
			report.EarliestDisplay.Layer != report.Earliest.Layer) {
		fmt.Fprintf(w, "最早显示时间线异常：帧 %s，%s / %s。\n",
			report.EarliestDisplay.FrameKey, layerName(report.EarliestDisplay.Layer),
			report.EarliestDisplay.Check)
	}

	if len(report.EarliestByLayer) > 0 {
		fmt.Fprintln(w, "分层最早失败：")
		for _, layer := range []check.Layer{check.LayerContainer, check.LayerBitstream, check.LayerDecode, check.LayerPixels} {
			if earliest, ok := report.EarliestByLayer[layer]; ok {
				fmt.Fprintf(w, "  - %s：%s / %s（decode_order=%d）\n",
					layerName(layer), earliest.FrameKey, earliest.Check, earliest.DecodeOrder)
			}
		}
	}

	fmt.Fprintln(w, "逐帧结论：")
	for _, verdict := range report.FrameVerdicts {
		fmt.Fprintf(w, "  - %s [%s] 重构判定=%s\n",
			verdict.FrameKey, layerSummary(verdict.LayerStatus), verdict.Reconstructable)
		if verdict.Mode != "" {
			fmt.Fprintf(w, "    方式：%s\n", modeName(verdict.Mode))
		}
		for _, reason := range verdict.Reasons {
			fmt.Fprintf(w, "    依据：%s\n", reason)
		}
		for _, warning := range verdict.Warnings {
			fmt.Fprintf(w, "    警告：%s\n", warning)
		}
		for _, blocker := range verdict.Blockers {
			fmt.Fprintf(w, "    阻塞：%s\n", blocker)
		}
	}

	if len(report.GlobalWarnings) > 0 {
		fmt.Fprintln(w, "全局警告：")
		for _, warning := range report.GlobalWarnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
}

func layerSummary(statuses map[check.Layer]check.Status) string {
	order := []check.Layer{check.LayerContainer, check.LayerBitstream, check.LayerDecode, check.LayerPixels}
	parts := make([]string, 0, len(order))
	for _, layer := range order {
		parts = append(parts, string(layer)+"="+string(statuses[layer]))
	}
	return strings.Join(parts, ", ")
}

func layerName(layer check.Layer) string {
	switch layer {
	case check.LayerContainer:
		return "容器元数据"
	case check.LayerBitstream:
		return "码流 CRC"
	case check.LayerDecode:
		return "解码结果"
	case check.LayerPixels:
		return "像素校验"
	default:
		return string(layer)
	}
}

func modeName(mode string) string {
	names := map[string]string{
		"copy_independent_known_good_pixels":                "复制独立可信原始像素",
		"redecode_known_good_packet_with_bit_exact_decoder": "用可信码包和精确解码器重解",
		"preserve_already_verified_pixels":                  "保留已通过可信校验的像素并修复容器",
		"current_pixels_failed_trusted_verification":        "当前像素未通过可信基准校验",
		"decode_failed_without_trusted_output":              "解码失败且无可信输出",
		"good_packet_needs_decoder_attestation":             "有好码包但缺少解码精确性保证",
		"needs_trusted_pixel_or_source_evidence":            "需要可信像素基准或独立好源",
	}
	if name, ok := names[mode]; ok {
		return name
	}
	return mode
}
