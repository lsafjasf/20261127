package check

import (
	"fmt"
	"strings"
)

func RenderText(report *Report) string {
	var out strings.Builder
	fmt.Fprintf(&out, "ProRes/Alpha 元数据与像素一致性报告\n")
	fmt.Fprintf(&out, "结论: %s — %s\n", strings.ToUpper(report.Status), report.Summary)
	if report.Source != nil {
		fmt.Fprintf(&out, "基准源: %s\n", report.Source.Path)
		writeFileReport(&out, "  ", *report.Source)
	}
	if report.RepairedVideo != "" {
		fmt.Fprintf(&out, "整段修复代理: %s\n", report.RepairedVideo)
	}
	if report.RepairScript != "" {
		fmt.Fprintf(&out, "修复脚本: %s\n", report.RepairScript)
	}

	if len(report.WrongParameters) > 0 {
		fmt.Fprintf(&out, "\n错误/风险参数 (%d):\n", len(report.WrongParameters))
		for i, param := range report.WrongParameters {
			frame := ""
			if param.Frame != nil {
				frame = fmt.Sprintf(" frame=%d", *param.Frame)
			}
			fmt.Fprintf(&out, "%d. [%s/%s]%s %s\n", i+1, param.Severity, param.Code, frame, param.Parameter)
			fmt.Fprintf(&out, "   文件: %s\n", param.File)
			if param.Expected != "" {
				fmt.Fprintf(&out, "   应为: %s\n", param.Expected)
			}
			if param.Actual != "" {
				fmt.Fprintf(&out, "   实际: %s\n", param.Actual)
			}
			fmt.Fprintf(&out, "   说明: %s\n", param.Message)
			if len(param.FFmpegArguments) > 0 {
				fmt.Fprintf(&out, "   修复参数: ffmpeg ... %s\n", strings.Join(param.FFmpegArguments, " "))
			}
			if param.RepairAction != "" {
				fmt.Fprintf(&out, "   帧修复: %s\n", param.RepairAction)
			}
		}
	}

	for _, variant := range report.Variants {
		fmt.Fprintf(&out, "\n变体: %s\n", variant.File.Path)
		writeFileReport(&out, "  ", variant.File)
		for _, frame := range variant.Frames {
			fmt.Fprintf(&out, "  帧 %d: alpha min=%d max=%d 非不透明=%d/%d, 判定=%s, 挂回源Alpha=%v, 反预乘=%v\n",
				frame.Index, frame.VariantAlpha.Min, frame.VariantAlpha.Max,
				frame.VariantAlpha.NonOpaque, frame.VariantAlpha.Pixels,
				frame.VariantModel, frame.AttachSourceAlpha, frame.Unpremultiply)
			if repaired, ok := frame.Outputs["repaired_rgba_png"]; ok {
				fmt.Fprintf(&out, "    修复帧: %s\n", repaired)
			}
			if checker, ok := frame.Outputs["checker_before_after"]; ok {
				fmt.Fprintf(&out, "    前后棋盘对照: %s\n", checker)
			}
		}
	}
	return out.String()
}

func writeFileReport(out *strings.Builder, indent string, file FileReport) {
	alphaType := "absent"
	if file.ProResAlphaType != nil {
		alphaType = fmt.Sprint(*file.ProResAlphaType)
	}
	fmt.Fprintf(out, "%s编码: %s profile=%s pix_fmt=%s ProRes alpha_type=%s 尺寸=%dx%d\n",
		indent, file.Codec, file.Profile, file.PixelFormat, alphaType, file.Width, file.Height)
	fmt.Fprintf(out, "%s颜色: primaries=%s transfer=%s matrix=%s range=%s\n",
		indent, dash(file.ColorPrimaries), dash(file.ColorTransfer), dash(file.ColorMatrix), dash(file.ColorRange))
}

func dash(value string) string {
	if value == "" {
		return "unspecified"
	}
	return value
}
