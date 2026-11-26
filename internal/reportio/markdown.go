package reportio

import (
	"clipforensic/internal/analyze"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteMarkdown(path string, report analyze.Report, svgPaths []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# 削波取证报告\n\n")
	fmt.Fprintf(&b, "- 文件数：`%d`；事故事件：`%d`；艺术性失真区间：`%d`；待定：`%d`\n",
		report.Summary.FilesAnalyzed, report.Summary.IncidentCandidates,
		report.Summary.ArtisticRegions, report.Summary.InconclusiveRegions)
	fmt.Fprintf(&b, "- 总结论：`%s`\n\n", report.Verdict)

	svgByID := map[string]string{}
	for _, p := range svgPaths {
		base := p
		if i := strings.LastIndex(p, "/"); i >= 0 {
			base = p[i+1:]
		}
		parts := strings.SplitN(base, "_", 3)
		if len(parts) >= 2 {
			id := parts[1]
			svgByID[id] = "waveforms/" + base
		}
	}

	for _, f := range report.Files {
		fmt.Fprintf(&b, "## `%s`\n\n", f.File)
		if f.Label != "" {
			fmt.Fprintf(&b, "标签：%s\n\n", f.Label)
		}
		fmt.Fprintf(&b, "`%d Hz`, `%d` 声道, `%.3f s`；峰值 `%.6f` (`%.2f dBFS`)，精确满刻度样本 `%d`，平台段 `%d` / 平台样本 `%d`。\n\n",
			f.SampleRate, f.Channels, f.DurationSec, f.Global.PeakAmplitude, f.Global.PeakDBFS,
			f.Global.ExactFullScaleSamples, f.Global.PlateauRunCount, f.Global.PlateauSamples)
		if len(f.Candidates) > 0 {
			b.WriteString("### 事故候选\n\n")
			b.WriteString("| ID | 时间(s) | 置信度 | 分类 | 平台(段/最长/FS样本) | 陡沿 | 频谱 | 证据 |\n")
			b.WriteString("|---|---:|---:|---|---:|---:|---|---|\n")
			for _, c := range f.Candidates {
				spec := fmt.Sprintf("centroid %.0fHz; 4k+ %.3f%%; splatter %+.1fdB; f0 %.0f; H %.2f",
					c.Spectrum.CentroidHz, 100*(c.Spectrum.HighRatio+c.Spectrum.AirRatio),
					c.Spectrum.SplatterDeltaDB, c.Spectrum.EstimatedF0Hz, c.Spectrum.HarmonicStrength)
				link := ""
				if p, ok := svgByID[c.ID]; ok {
					link = "[SVG](" + p + ")"
				}
				fmt.Fprintf(&b, "| %s | %.3f–%.3f | %.2f | `%s` | %d/%d/%d | %.2f | %s | %s |\n",
					c.ID, c.StartSec, c.EndSec, c.Confidence, c.Classification,
					c.Plateau.RunCount, c.Plateau.LongestRun, c.Plateau.ExactFullScaleSamples,
					c.Edge.SteepEdgeRatio, spec, link)
			}
			b.WriteString("\n")
			for _, c := range f.Candidates {
				fmt.Fprintf(&b, "- **%s %.3f–%.3f s**：%s\n", c.ID, c.StartSec, c.EndSec, c.Attribution)
				for _, r := range c.Reasons {
					fmt.Fprintf(&b, "  - %s\n", r)
				}
				if c.Comparison != nil {
					fmt.Fprintf(&b, "  - 母带比对：`%s`；参考平台段 `%d`，新增平台段 `%d`，对齐延迟 `%d` samples。\n",
						c.Comparison.Verdict, c.Comparison.ReferencePlateauRuns,
						c.Comparison.NewPlateauRuns, c.Comparison.AlignedLagSamples)
				}
			}
			b.WriteString("\n")
		}
		if len(f.Artistic) > 0 {
			b.WriteString("### 艺术性失真/待定保护区间\n\n")
			b.WriteString("| ID | 时间(s) | 状态 | 分类 | 置信度 | f0/谐波 | 说明 | 证据 |\n")
			b.WriteString("|---|---:|---|---|---:|---|---|---|\n")
			for _, c := range f.Artistic {
				link := ""
				if p, ok := svgByID[c.ID]; ok {
					link = "[SVG](" + p + ")"
				}
				fmt.Fprintf(&b, "| %s | %.3f–%.3f | `%s` | `%s` | %.2f | %.1fHz / %.2f | %s | %s |\n",
					c.ID, c.StartSec, c.EndSec, c.Status, c.Classification, c.Confidence,
					c.Spectrum.EstimatedF0Hz, c.Spectrum.HarmonicStrength, c.Attribution, link)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## 归因边界\n\n")
	b.WriteString("- PCM 中大量精确同值/满刻度平台、1–2 sample 陡直上升沿和宽带高频泄漏，指向母带总线限幅或硬削波。\n")
	b.WriteString("- 只有分发版、没有编码前母带时，不能从信号本身证明“编码算法”生成了平台；只能给出与母带削波一致的置信判断。\n")
	b.WriteString("- 提供编码前母带和解码后的平台文件可做样本对齐：母带同位置已有平台即母带/限幅器来源；母带干净而分发版新增圆角沿、过冲或 codec frame grid 对齐，才支持编码/转码引入。\n")
	b.WriteString("- 失真吉他按音头、稳定基频、谐波序列和缺乏数字天花板平台保护；高频多本身不是事故依据。\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
