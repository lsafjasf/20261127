package timemap

import (
	"fmt"
	"io"
	"math/big"
	"strings"
)

// WriteConsoleTable prints a compact, grep-friendly comparison table.
func WriteConsoleTable(w io.Writer, all [][]Pair) {
	fmt.Fprintln(w, "track    point                        edit_ms       ref_M   -> built_M  (dM)        ref_D -> built_D (dD)   first-bad-stage")
	fmt.Fprintln(w, strings.Repeat("-", 126))
	for _, pairs := range all {
		for _, p := range pairs {
			editMs := FormatSeconds(scaleMilli(p.Ref.Edit.Time), 3)
			fmt.Fprintf(w, "%-8s %-28s %-13s %8s -> %-8s (%+d)   %6s -> %-7s (%+d)   %s\n",
				p.Track,
				p.Point.Label,
				editMs,
				p.Ref.Media.Index.String(),
				p.Built.Media.Index.String(),
				p.DMed.Int64(),
				p.Ref.Display.Index.String(),
				p.Built.Display.Index.String(),
				p.DDisp.Int64(),
				p.Stage,
			)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "图例: media = 媒体时钟环节首次出错；media->display = 该行媒体索引恰好相同，但显示槽差异由该轨道媒体环节的根因向下游传播；display = 显示环节原生错误。")
}

// WriteMarkdown emits the findings report with reproducible evidence.
func WriteMarkdown(w io.Writer, edit EditResolution, refresh Rate, tracks []TrackConfig, points []TimelinePoint, findings []Finding) {
	fmt.Fprintln(w, "# 三时钟映射与一帧错位定位报告")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## 时间域定义")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- 编辑时间 (edit time): 编辑器绝对时间轴，时间基 `1/%d s`（1 tick = 1 µs），所有轨道共用同一绝对时间戳。\n", edit.TicksPerSecond)
	fmt.Fprintln(w, "- 媒体时间 (media time): 媒体自身时钟上的整数单元，视频为帧号、音频为采样号；`index = floor(edit_seconds × media_rate)`。")
	fmt.Fprintf(w, "- 显示时间 (display time): 输出端 vsync 调度槽，刷新率 `%s Hz`；`slot = floor(media_seconds × refresh_rate)`。\n", rateLabel(refresh))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "参考管线全程使用 `math/big.Rat` 精确有理运算 + floor；实际管线使用 float64 与各轨道配置的缺陷规则。")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## 轨道配置")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| 轨道 | 真实媒体速率 | 实际使用速率 | 媒体舍入 | 显示舍入 |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, c := range tracks {
		builtRate := rateLabel(c.MediaRate)
		if c.BuiltMediaRate != nil {
			builtRate = rateLabel(*c.BuiltMediaRate)
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
			c.Name, rateLabel(c.MediaRate), builtRate, c.BuiltMediaRule, c.BuiltDisplayRule)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## 定位结论")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| 轨道 | 首个出错环节 | 根因 | 首个分歧点 | 首点 delta（实际−参考） | 分歧点数/总点数 | 末点漂移 |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|")
	for _, f := range findings {
		first := "-"
		delta := "-"
		if f.FirstDivergence != nil {
			first = f.FirstDivergence.Label
			delta = f.DeltaAtFirst.String()
		}
		drift := "-"
		if f.DriftAtLast != nil {
			drift = f.DriftAtLast.String()
		}
		fmt.Fprintf(w, "| %s | %s | `%s` | %s | %s | %d/%d | %s |\n",
			f.Track, f.Stage, f.Cause, first, delta, f.DivergentPoints, f.TotalPoints, drift)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## 逐轨道证据与修复")
	fmt.Fprintln(w)
	for _, f := range findings {
		fmt.Fprintf(w, "### %s\n\n", f.Track)
		fmt.Fprintf(w, "- 结论: 环节 `%s`，根因 `%s`。\n", f.Stage, f.Cause)
		fmt.Fprintf(w, "- 判定: %s。\n", f.CauseCN())
		fmt.Fprintf(w, "- 证据: %s。\n", detailCN(f))
		if f.FirstDivergence != nil {
			fmt.Fprintf(w, "- 首个分歧: `%s`（tick %d），实际索引−参考索引 = %s 个媒体单元。\n",
				f.FirstDivergence.Label, f.FirstDivergence.Tick, f.DeltaAtFirst.String())
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "## 复核方法")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "1. CSV 宽表 `timemap.csv` 逐行给出精确分数索引、float64 缩放值、整数索引与逐级 delta。")
	fmt.Fprintln(w, "2. 任意一行可用手算复核: `index = floor(tick/1e6 × rate_num/rate_den)`，显示槽同理。")
	fmt.Fprintln(w, "3. 修复验证: 将实际管线的速率改回真实媒体速率、舍入统一为 floor，重跑程序，所有 delta 列应为 0。")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "检查的时间点共 %d 个: %s。\n", len(points), pointLabels(points))
}

func detailCN(f Finding) string {
	switch f.Cause {
	case CauseRounding:
		return "在该环节对 时间×速率 的结果使用 nearest 舍入；改用 floor 后所有实际索引均可精确复现，分歧恰好为 1 个媒体单元"
	case CauseWrongRate:
		return "该环节按错误速率缩放时间；改用真实媒体速率后索引与参考一致，且漂移随时间累积"
	case CauseOffByOne:
		return "edit time 为 0 时索引已偏移 1，属于帧号/采样号从 1 开始编号而非从 0 开始"
	case CauseCompound:
		return "错误速率与错误舍入同时存在，需要同时修正"
	case CauseNone:
		return "实际管线在各环节均与精确参考一致"
	default:
		return f.Detail
	}
}

func scaleMilli(r *big.Rat) *big.Rat {
	return new(big.Rat).Mul(r, big.NewRat(1000, 1))
}

func pointLabels(points []TimelinePoint) string {
	labels := make([]string, len(points))
	for i, p := range points {
		labels[i] = p.Label
	}
	return strings.Join(labels, ", ")
}
