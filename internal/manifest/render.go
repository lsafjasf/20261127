package manifest

import (
	"encoding/json"
	"fmt"
	"strings"
)

func MarshalJSON(report Report) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return append(data, '\n'), nil
}

func RenderMarkdown(report Report) []byte {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# HDR/SDR 双版本交付清单\n\n")
	fmt.Fprintf(&builder, "- 项目：%s\n", markdownValue(report.Project.ID))
	if report.Project.Title != "" {
		fmt.Fprintf(&builder, "- 标题：%s\n", markdownValue(report.Project.Title))
	}
	fmt.Fprintf(&builder, "- 生成时间：%s\n", report.GeneratedAt)
	fmt.Fprintf(&builder, "- 生成器：%s %s\n", report.Generator.Name, report.Generator.Version)
	fmt.Fprintf(&builder, "- 总体状态：**%s**\n\n", statusLabel(report.Status))

	fmt.Fprintf(&builder, "## 总览\n\n")
	fmt.Fprintf(&builder, "| 范围 | 状态 |\n|---|---|\n")
	fmt.Fprintf(&builder, "| HDR | %s |\n", statusLabel(report.Summary.HDRStatus))
	fmt.Fprintf(&builder, "| SDR | %s |\n", statusLabel(report.Summary.SDRStatus))
	fmt.Fprintf(&builder, "| 共享项 | %s |\n", statusLabel(report.Summary.SharedStatus))
	fmt.Fprintf(&builder, "| 检查项 | %d 通过 / %d 失败 / 共 %d |\n\n",
		report.Summary.PassedChecks,
		report.Summary.FailedChecks,
		report.Summary.TotalChecks,
	)

	if len(report.Failures) > 0 {
		fmt.Fprintf(&builder, "## 未通过项\n\n")
		fmt.Fprintf(&builder, "| 范围 | 检查项 | 说明 | 期望 | 实际 |\n|---|---|---|---|---|\n")
		for _, failure := range report.Failures {
			fmt.Fprintf(&builder, "| %s | `%s` | %s | %s | %s |\n",
				markdownValue(failure.Scope),
				markdownValue(failure.CheckID),
				markdownValue(failure.Detail),
				markdownValue(formatValue(failure.Expected)),
				markdownValue(formatValue(failure.Actual)),
			)
		}
		builder.WriteString("\n")
	}

	writeTimelineMarkdown(&builder, report.Shared.Timeline)
	writeVersionMarkdown(&builder, report.Versions.HDR)
	writeVersionMarkdown(&builder, report.Versions.SDR)
	writeSharedAssetsMarkdown(&builder, report.Shared)
	writeChecksMarkdown(&builder, report)
	return []byte(builder.String())
}

func writeTimelineMarkdown(builder *strings.Builder, timeline TimelineReport) {
	fmt.Fprintf(builder, "## 共享时间线\n\n")
	fmt.Fprintf(builder, "| 版本 | 时长 | 起点 | 帧率 | 估算帧数 | 时长来源 |\n|---|---:|---:|---:|---:|---|\n")
	for _, item := range []struct {
		role   string
		actual TimelineActual
	}{
		{role: "HDR", actual: timeline.HDR},
		{role: "SDR", actual: timeline.SDR},
	} {
		fmt.Fprintf(builder, "| %s | %s | %s | %s | %s | %s |\n",
			item.role,
			optionalValue(item.actual.DurationAvailable, fmt.Sprintf("%d ms", item.actual.DurationMS)),
			optionalValue(item.actual.StartAvailable, fmt.Sprintf("%d ms", item.actual.StartMS)),
			optionalValue(item.actual.FrameRateAvailable, fmt.Sprintf("%.6g fps", item.actual.FrameRate)),
			frameCountText(item.actual),
			markdownValue(item.actual.DurationSource),
		)
	}
	builder.WriteString("\n")
}

func frameCountText(actual TimelineActual) string {
	if actual.FrameCountAvailable {
		return fmt.Sprintf("%d（精确）", actual.FrameCount)
	}
	if actual.EstimatedFrames > 0 {
		return fmt.Sprintf("~%d（估算）", actual.EstimatedFrames)
	}
	return "缺失"
}

func writeVersionMarkdown(builder *strings.Builder, version VersionReport) {
	fmt.Fprintf(builder, "## %s\n\n", version.Label)
	fmt.Fprintf(builder, "- 文件：`%s`\n", version.Path)
	fmt.Fprintf(builder, "- 状态：%s\n", statusLabel(version.Status))
	if version.FileSHA256 != "" {
		fmt.Fprintf(builder, "- 文件 SHA-256：`%s`\n", version.FileSHA256)
	}
	fmt.Fprintf(builder, "- 色彩标准：%s\n", markdownValue(version.Color.Standard))
	fmt.Fprintf(builder, "- 实际色彩：primaries=%s, transfer=%s, matrix=%s, range=%s, bit_depth=%s\n",
		markdownValue(version.Color.Actual.Primaries),
		markdownValue(version.Color.Actual.Transfer),
		markdownValue(version.Color.Actual.Matrix),
		markdownValue(version.Color.Actual.Range),
		bitDepthText(version.Color.Actual.BitDepth),
	)
	fmt.Fprintf(builder, "- 主控显示元数据：%s；内容光级：%s",
		boolText(version.Color.Actual.MasteringDisplay),
		boolText(version.Color.Actual.ContentLight),
	)
	if version.Color.Actual.ContentLight {
		fmt.Fprintf(builder, "（MaxCLL=%d, MaxFALL=%d）", version.Color.Actual.MaxCLL, version.Color.Actual.MaxFALL)
	}
	builder.WriteString("\n")

	fmt.Fprintf(builder, "- 切点：模式=%s，期望=%d，实际=%d，缺失=%d，多余=%d\n",
		version.Cuts.Mode,
		len(version.Cuts.Expected),
		len(version.Cuts.Actual),
		len(version.Cuts.Missing),
		len(version.Cuts.Unexpected),
	)
	fmt.Fprintf(builder, "- 默认音轨：%s\n", trackText(version.Defaults.Audio))
	fmt.Fprintf(builder, "- 默认字幕：%s\n\n", trackText(version.Defaults.Subtitle))
}

func writeSharedAssetsMarkdown(builder *strings.Builder, shared SharedReport) {
	fmt.Fprintf(builder, "## 共享资产\n\n")
	if len(shared.Assets) == 0 {
		fmt.Fprintf(builder, "未声明外部共享资产文件；已按容器内音轨/字幕流哈希对拍。\n\n")
		return
	}
	fmt.Fprintf(builder, "| ID | 类型 | 状态 | SHA-256 | 流哈希 | HDR | SDR | 默认 |\n|---|---|---|---|---|---|---|---|\n")
	for _, asset := range shared.Assets {
		fmt.Fprintf(builder, "| `%s` | %s | %s | `%s` | `%s` | %s | %s | %s |\n",
			markdownValue(asset.ID),
			markdownValue(asset.Kind),
			statusLabel(asset.Status),
			markdownValue(shortHash(asset.SHA256)),
			markdownValue(shortHash(asset.StreamSHA256)),
			embeddedRefText(asset.HDR),
			embeddedRefText(asset.SDR),
			declaredDefaultText(asset.DeclaredDefault),
		)
	}
	builder.WriteString("\n")
}

func writeChecksMarkdown(builder *strings.Builder, report Report) {
	fmt.Fprintf(builder, "## 逐项检查\n\n")
	fmt.Fprintf(builder, "| 范围 | 分类 | 检查项 | 结果 | 期望 | 实际 | 说明 |\n|---|---|---|---|---|---|---|\n")
	for _, item := range collectAllChecks(&report) {
		result := "通过"
		if !item.Check.Pass {
			result = "失败"
		}
		fmt.Fprintf(builder, "| %s | %s | `%s` | %s | %s | %s | %s |\n",
			markdownValue(item.Scope),
			markdownValue(item.Check.Category),
			markdownValue(item.Check.ID),
			result,
			markdownValue(formatValue(item.Check.Expected)),
			markdownValue(formatValue(item.Check.Actual)),
			markdownValue(item.Check.Detail),
		)
	}
}

func statusLabel(status string) string {
	if status == StatusComplete {
		return "完整"
	}
	return "不完整"
}

func optionalValue(available bool, value string) string {
	if !available {
		return "缺失"
	}
	return value
}

func trackText(track *TrackSummary) string {
	if track == nil {
		return "缺失"
	}
	return fmt.Sprintf("#%d %s / %s / %s / default=%t / `%s`",
		track.Index,
		markdownValue(track.Language),
		markdownValue(track.Title),
		markdownValue(track.Codec),
		track.Default,
		shortHash(track.StreamSHA256),
	)
}

func embeddedRefText(ref *EmbeddedRef) string {
	if ref == nil || !ref.Found {
		return "缺失"
	}
	return fmt.Sprintf("#%d", ref.Index)
}

func declaredDefaultText(value *bool) string {
	if value == nil {
		return "未声明"
	}
	return fmt.Sprintf("%t", *value)
}

func shortHash(hash string) string {
	if hash == "" {
		return "缺失"
	}
	if len(hash) <= 16 {
		return hash
	}
	return hash[:16] + "…"
}

func markdownValue(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func formatValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case []string:
		return strings.Join(typed, ", ")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}
