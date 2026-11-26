package pipeline

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"colorfix/internal/color"
)

type Report struct {
	Schema      string       `json:"schema"`
	GeneratedAt string       `json:"generated_at"`
	Files       []FileReport `json:"files"`
}

type pageData struct {
	GeneratedAt string
	Files       []FileReport
}

func WriteReport(outDir string, files []FileReport) error {
	report := Report{
		Schema:      "colorfix/report/v1",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Files:       files,
	}
	if err := writeJSON(filepath.Join(outDir, "report.json"), report); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "report.md"), []byte(markdown(report)), 0o644); err != nil {
		return err
	}
	return writeIndex(filepath.Join(outDir, "index.html"), report)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

var statusText = map[string]string{
	"verified":              "已验证，无需更正",
	"relabeled":             "已唯一定位并更正标签",
	"labeled":               "已按测量结果补标",
	"ambiguous":             "证据接近，保留候选",
	"insufficient_evidence": "证据不足，未强制更正",
}

func markdown(report Report) string {
	var b strings.Builder
	b.WriteString("# 矩阵标签核查报告\n\n")
	b.WriteString("判定依据：彩条结构、BT.601/BT.709 色条残差、容器矩阵声明。只有残差差距明确时才写入修正标签；否则保留两个候选。\n\n")
	fmt.Fprintf(&b, "生成时间：`%s`\n\n", report.GeneratedAt)
	b.WriteString("| 输入 | 状态 | 声明 | 结果 | 601 残差 | 709 残差 | 修正媒体 |\n")
	b.WriteString("|---|---|---|---|---:|---:|---|\n")
	for _, file := range report.Files {
		status := statusText[file.Decision.Status]
		if status == "" {
			status = file.Decision.Status
		}
		selected := string(file.Decision.Selected)
		if selected == "" {
			var names []string
			for _, c := range file.Decision.Candidates {
				names = append(names, string(c.Matrix))
			}
			selected = "候选: " + strings.Join(names, " / ")
		}
		mediaPath := "—"
		if file.Fixed != nil {
			mediaPath = filepath.Base(file.Fixed.Path)
		}
		if file.Error != "" {
			status = "处理失败"
			selected = file.Error
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %.4f | %.4f | %s |\n",
			filepath.Base(file.Input), status, emptyDash(file.Labels.DeclaredName),
			selected, file.Bars.Score(color.BT601).Residual, file.Bars.Score(color.BT709).Residual,
			mediaPath)
	}
	b.WriteString("\n## 对照帧\n\n")
	for _, file := range report.Files {
		if file.BarsFrame == nil && file.Content == nil {
			continue
		}
		fmt.Fprintf(&b, "### `%s`\n\n", filepath.Base(file.Input))
		if file.BarsFrame != nil {
			fmt.Fprintf(&b, "- 彩条帧：`%s`（时间 %.3fs）\n", filepath.Join(filepath.Base(file.OutputDir), file.BarsFrame.Frames.Compare), file.BarsFrame.Time)
		}
		if file.Content != nil {
			fmt.Fprintf(&b, "- 内容帧：`%s`（时间 %.3fs）\n", filepath.Join(filepath.Base(file.OutputDir), file.Content.Frames.Compare), file.Content.Time)
		}
		fmt.Fprintf(&b, "- 标签文件：`%s`\n\n", filepath.Join(filepath.Base(file.OutputDir), "label.json"))
	}
	return b.String()
}

func writeIndex(path string, report Report) error {
	funcs := template.FuncMap{
		"status": func(s string) string {
			if text := statusText[s]; text != "" {
				return text
			}
			return s
		},
		"base": filepath.Base,
		"join": filepath.Join,
		"pct":  func(f float64) string { return fmt.Sprintf("%.0f%%", f*100) },
	}
	tpl, err := template.New("index").Funcs(funcs).Parse(indexTemplate)
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return tpl.Execute(file, pageData{GeneratedAt: report.GeneratedAt, Files: report.Files})
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
