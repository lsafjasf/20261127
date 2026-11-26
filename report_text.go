package main

import (
	"fmt"
	"sort"
	"strings"
)

// RenderText renders a human-readable Chinese audit report.
func RenderText(rep *Report, allWindows bool) string {
	var b strings.Builder
	b.WriteString("framedoctor 直播帧异常审计报告\n")
	b.WriteString(strings.Repeat("=", 78) + "\n")
	fmt.Fprintf(&b, "时间窗 %.2fs | 重复阈值 dup<=%vbit | 跳变 >=%.1fx基线\n",
		rep.WindowSec, rep.Thresholds["dup_bits"], rep.Thresholds["jump_ratio"])

	b.WriteString("\n[输入轨迹]\n")
	if len(rep.Traces) == 0 {
		b.WriteString("  (无)\n")
	}
	for _, s := range rep.Traces {
		fmt.Fprintf(&b, "  %-4s %-42s 帧=%d", StageLabel(s.Stage), truncate(s.Source, 42), s.Frames)
		if s.Markers > 0 {
			fmt.Fprintf(&b, "(+%d丢帧标记)", s.Markers)
		}
		fmt.Fprintf(&b, " fps=%.1f 轴=%s 跨度=%.2fs 签名=%d\n",
			s.FPS, axisLabel(s.Axis), s.SpanS, s.WithSig)
	}

	for _, st := range []Stage{StageCapture, StageRecord, StagePlayer} {
		wins, ok := rep.Windows[string(st)]
		if !ok {
			continue
		}
		b.WriteString("\n[窗口账·" + StageLabel(st) + "] 应有/实际/画面推进\n")
		renderWindowTable(&b, wins, allWindows)
	}

	b.WriteString("\n[异常结论]\n")
	anomalyN := 0
	for _, f := range rep.Findings {
		if f.Anomaly && f.Severity != "info" {
			anomalyN++
		}
	}
	if anomalyN == 0 {
		b.WriteString("  ✓ 未发现采集丢帧 / 编码重复 / 播放器跳帧\n")
	} else {
		fmt.Fprintf(&b, "  共识别 %d 个异常项", anomalyN)
		if rep.First != nil {
			fmt.Fprintf(&b, "，首个异常 → %s @ %.3fs（%s，置信度%.0f%%）\n",
				rep.First.Title, firstTime(rep.First), severityCN(rep.First.Severity),
				rep.First.Confidence*100)
		} else {
			b.WriteString("\n")
		}
	}
	for _, f := range rep.Findings {
		renderFinding(&b, f, f == rep.First)
	}
	if len(rep.Notes) > 0 {
		b.WriteString("\n[备注]\n")
		for _, n := range rep.Notes {
			b.WriteString("  - " + n + "\n")
		}
	}
	return b.String()
}

func firstTime(f *Finding) float64 {
	if f.PTS != nil {
		return *f.PTS
	}
	return f.TimeS
}

func axisLabel(a string) string {
	if a == "wall" {
		return "墙钟"
	}
	if a == "pts" {
		return "PTS"
	}
	return a
}

func severityCN(s string) string {
	switch s {
	case "confirmed":
		return "确认"
	case "likely":
		return "高度可能"
	case "suspected":
		return "疑似"
	case "signal":
		return "信号"
	case "info":
		return "说明"
	}
	return s
}

func renderWindowTable(b *strings.Builder, wins []*Window, all bool) {
	fmt.Fprintf(b, "  %4s %7s %5s %5s %5s %5s %4s %4s %7s  %s\n",
		"WIN", "START", "WANT", "GOT", "MISS", "FRESH", "DUP", "JUMP", "ADV_U", "STATUS")
	shown := 0
	for _, w := range wins {
		anomalous := w.Status != "ok"
		if !all && len(wins) > 60 && !anomalous {
			continue
		}
		shown++
		fmt.Fprintf(b, "  %4d %7.2f %5.0f %5d %5.0f %5d %4d %4d %7.1f  %s\n",
			w.Index, w.Start, w.Want, w.Got, w.Missing, w.Fresh, w.Dup, w.Jumps,
			w.AdvanceUnits, statusCN(w.Status))
	}
	if !all && len(wins) > 60 {
		fmt.Fprintf(b, "  ... 仅显示异常窗口（共 %d 窗，加 --all-windows 查看全部）\n", len(wins))
	}
	if shown == 0 {
		b.WriteString("  （全部窗口正常）\n")
	}
}

func statusCN(status string) string {
	if status == "ok" {
		return "ok"
	}
	parts := strings.Split(status, "+")
	cn := map[string]string{
		"frame_loss": "缺帧", "duplicate": "重复", "jump": "跳变",
	}
	out := []string{}
	for _, p := range parts {
		if c, ok := cn[p]; ok {
			out = append(out, c)
		}
	}
	return strings.Join(out, "+")
}

func renderFinding(b *strings.Builder, f *Finding, first bool) {
	marker := "  "
	if first {
		marker = "★ "
	}
	loc := fmt.Sprintf("window=%d t=%.3fs", f.Window, f.TimeS)
	if f.PTS != nil {
		loc += fmt.Sprintf(" pts=%.3fs", *f.PTS)
	}
	if f.HasSeq {
		loc += fmt.Sprintf(" seq=%d", f.Seq)
	}
	if f.Frames > 0 && (f.Kind == KindCaptureDrop || f.Kind == KindPlayerSkip ||
		f.Kind == KindFrameLoss || f.Kind == KindPlayerShort || f.Kind == KindEncodeDrop) {
		loc += fmt.Sprintf(" n≈%d", f.Frames)
	}
	fmt.Fprintf(b, "%s[%s][%s·%s %.0f%%] %s\n      %s\n",
		marker, f.ID, f.Title, severityCN(f.Severity), f.Confidence*100, loc, evidenceLine(f.Evidence))
}

var evidenceCN = map[string]string{
	"want": "应有", "got": "实际", "missing_est": "估算缺帧", "marked_dropped": "显式丢帧标记",
	"fresh": "新鲜帧数", "advance_frame_units": "推进当量", "extra_copies": "额外复制份",
	"motion_baseline": "运动基线", "edge_norm": "边距离", "jump_at_s": "跳变时刻",
	"jump_norm": "跳变量", "jump_ratio": "跳变倍数", "mapped_record_pts": "映射录制PTS",
	"mapped_capture_seq": "映射采集帧号", "record_copies": "录制复制数",
	"skipped_record_frames": "跳过录制帧", "first_skipped_index": "首跳下标",
	"skipped_seq_sample": "跳过帧号样本", "resume_at_record_idx": "恢复下标",
	"dropped_seq": "丢帧号", "ratio_to_baseline": "基线倍数",
}

func evidenceLine(ev map[string]any) string {
	if len(ev) == 0 {
		return ""
	}
	keys := make([]string, 0, len(ev))
	for k := range ev {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		label := evidenceCN[k]
		if label == "" {
			label = k
		}
		parts = append(parts, fmt.Sprintf("%s=%v", label, formatEv(ev[k])))
	}
	return "证据: " + strings.Join(parts, " ")
}

func formatEv(v any) string {
	switch x := v.(type) {
	case []float64:
		ss := []string{}
		for _, f := range x {
			ss = append(ss, fmt.Sprintf("%.3f", f))
		}
		return "[" + strings.Join(ss, ",") + "]"
	case []int64:
		ss := []string{}
		for _, n := range x {
			ss = append(ss, fmt.Sprintf("%d", n))
		}
		return "[" + strings.Join(ss, ",") + "]"
	default:
		return fmt.Sprintf("%v", x)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n+3:]
}
