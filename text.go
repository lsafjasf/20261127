package tailtrace

import (
	"fmt"
	"strings"
)

// RenderText renders a concise diagnosis suitable for a terminal or issue
// comment. Machine-readable output is available through JSON in the CLI.
func RenderText(report *Report) string {
	var out strings.Builder
	d := report.Durations

	fmt.Fprintf(&out, "结论: %s\n", report.Classification)
	fmt.Fprintf(&out, "%s\n\n", report.Summary)

	fmt.Fprintln(&out, "时间账:")
	fmt.Fprintf(&out, "  CFR 帧时长        : %s (%s)\n", d.FrameDuration.SecondsText(6), d.FPS.String()+" fps")
	fmt.Fprintf(&out, "  内容帧/时长       : %d / %s\n", d.ContentFrames, d.ContentDuration.SecondsText(6))
	fmt.Fprintf(&out, "  输出帧/CFR 时长   : %d / %s\n", d.OutputFrames, d.CFRDuration.SecondsText(6))
	fmt.Fprintf(&out, "  末帧 PTS          : %s\n", d.LastPTS.SecondsText(6))
	fmt.Fprintf(&out, "  末帧显示结束      : %s (末帧 PTS + 一帧时长)\n", d.LastPTSEnd.SecondsText(6))
	fmt.Fprintf(&out, "  尾部黑场          : %d 帧 / %s\n", d.TailBlackFrames, d.TailBlackDuration.SecondsText(6))
	if d.TailBlackStartPTS != nil {
		fmt.Fprintf(&out, "  首个黑场 PTS      : %s (恰好等于内容时间线结束)\n", d.TailBlackStartPTS.SecondsText(6))
	}
	fmt.Fprintf(&out, "  合同帧/时长       : %d / %s\n", d.ContractFrames, d.ContractDuration.SecondsText(6))
	fmt.Fprintf(&out, "  播放器排期结束    : %s\n", d.PlayerScheduleEnd.SecondsText(6))
	if d.PlayerReportedEnd != nil {
		fmt.Fprintf(&out, "  播放器上报时长    : %s\n", d.PlayerReportedEnd.SecondsText(6))
	}
	if d.EditListEnd != nil {
		fmt.Fprintf(&out, "  EditList 结束     : %s\n", d.EditListEnd.SecondsText(6))
	}
	if d.ContainerDuration != nil {
		fmt.Fprintf(&out, "  容器 duration     : %s\n", d.ContainerDuration.SecondsText(6))
	}
	if d.AudioDuration != nil {
		fmt.Fprintf(&out, "  音频/母版时长     : %s\n", d.AudioDuration.SecondsText(6))
	}

	fmt.Fprintln(&out, "\nPTS/显示区间（内容末尾与尾部帧）:")
	fmt.Fprintln(&out, "  idx  role                 black  PTS(s)    drift(s)  display interval(s)")
	start := d.ContentFrames - 2
	if start < 0 {
		start = 0
	}
	if d.TailBlackFrames == 0 && len(report.Frames) > 8 {
		start = int64(len(report.Frames) - 5)
	}
	for position, frame := range report.Frames {
		if int64(position) < start {
			continue
		}
		black := "-"
		if frame.Black {
			black = "BLACK"
		}
		fmt.Fprintf(&out, "  %-4d %-20s %-6s %-9s %-9s [%.6f, %.6f)\n",
			frame.Index, frame.Role, black,
			frame.PTS.SecondsText(6), frame.PTSDrift.SecondsText(9),
			frame.DisplayStart.Float64(), frame.DisplayEnd.Float64(),
		)
	}

	fmt.Fprintln(&out, "\n核对结果:")
	for _, finding := range report.Findings {
		fmt.Fprintf(&out, "  [%s] %s: %s\n", finding.Severity, finding.Code, finding.Message)
	}

	return out.String()
}
