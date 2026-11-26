package analyzer

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteJSON writes the stable machine-readable report.
func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(report)
}

// WriteText writes a compact human-readable diagnosis.
func WriteText(w io.Writer, report Report) {
	fmt.Fprintln(w, "缩放文字软边诊断")
	fmt.Fprintf(w, "结论: %s (%s)\n", verdictLabel(report.Verdict), report.Verdict)
	fmt.Fprintf(w, "置信度: %.0f%%\n", report.Confidence*100)
	fmt.Fprintf(w, "摘要: %s\n\n", report.Summary)
	fmt.Fprintln(w, "证据:")
	for i, evidence := range report.Evidence {
		fmt.Fprintf(w, "  %d. %s\n", i+1, evidence.Text)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "序列:")
	printSequence(w, "源帧", report.Source)
	printSequence(w, "缩放后", report.Scaled)
	fmt.Fprintf(w, "归一化倍率: %.3f x %.3f (几何平均 %.3f)\n", report.Comparison.ScaleX, report.Comparison.ScaleY, report.Comparison.Scale)
	fmt.Fprintln(w, "提示: ROI 应只覆盖发生软边的文字区域；静态序列至少 3 帧更利于区分编码闪烁。")
}

func printSequence(w io.Writer, label string, stats SequenceStats) {
	fmt.Fprintf(w, "  %s: %dx%d, %d 帧\n", label, stats.Width, stats.Height, stats.Frames)
	fmt.Fprintf(w, "    边缘宽度 p50/p75: %.2f / %.2f px (%d 样本)\n",
		stats.Spatial.EdgeWidthMedian, stats.Spatial.EdgeWidthP75, stats.Spatial.EdgeSamples)
	fmt.Fprintf(w, "    高频归一化: %.2f, 梯度 RMS: %.2f, 梯度 p99: %.2f\n",
		stats.Spatial.HFNorm, stats.Spatial.Gradient.RMS, stats.Spatial.Gradient.P99)
	if stats.Block.Best.Size > 0 && stats.Block.Best.Phase >= 0 {
		fmt.Fprintf(w, "    最强块边界: %s/%dpx phase=%d, 超额 %.1f%%, z=%.1f, 置信度 %.2f\n",
			stats.Block.Best.Axis, stats.Block.Best.Size, stats.Block.Best.Phase,
			stats.Block.Best.Excess*100, stats.Block.Best.ZScore, stats.Block.Best.Confidence)
	} else {
		fmt.Fprintln(w, "    最强块边界: 未检测到")
	}
	fmt.Fprintf(w, "    帧间差异/边缘噪声(归一化): %.3f / %.3f\n",
		stats.Temporal.DeltaNorm, stats.Temporal.EdgeNoiseNorm)
}

func verdictLabel(v Verdict) string {
	switch v {
	case VerdictSource:
		return "源帧"
	case VerdictEncoding:
		return "编码"
	case VerdictDisplaySampling:
		return "显示采样"
	case VerdictSourceDisplay:
		return "源帧 + 显示采样"
	default:
		return "证据不足"
	}
}
