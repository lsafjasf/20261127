package laddercheck

import (
	"fmt"
	"strings"
)

// RenderMarkdown renders the full Chinese comparison + verdict report.
func RenderMarkdown(rep *Report) string {
	var b strings.Builder
	c := rep.Constraint

	fmt.Fprintf(&b, "# 转码 Ladder 统一关键帧核对报告\n\n")
	fmt.Fprintf(&b, "- Schema：`%s`\n", rep.Schema)
	if rep.Input != "" {
		fmt.Fprintf(&b, "- 输入：`%s`\n", rep.Input)
	}
	fmt.Fprintf(&b, "- 档位数：%d\n", rep.Summary.Renditions)
	fmt.Fprintf(&b, "- 当前是否可安全无缝切换（全部 compliant）：**%v**\n", rep.Summary.SwitchSafe)
	fmt.Fprintf(&b, "- 是否所有档都能“不重编码”对齐：**%v**\n\n", rep.Summary.AlignableWithoutReencode)

	b.WriteString("## 1. 结论：哪些档无法在不重编码的情况下满足\n\n")
	b.WriteString("| 分类 | 档位 | 含义 |\n| --- | --- | --- |\n")
	fmt.Fprintf(&b, "| ✅ compliant | %s | 关键帧/IDR/时间线已满足统一约束 |\n", mdList(rep.Summary.Compliant))
	fmt.Fprintf(&b, "| 🟡 remux-only | %s | 编码结构正确，仅编辑列表/首帧 PTS 等标签偏移；改封装即可，**无需重编码** |\n", mdList(rep.Summary.RemuxOnly))
	fmt.Fprintf(&b, "| 🔴 requires-reencode | %s | 统一切换点缺少 IDR 或关键帧不是闭合 IDR；**不重编码无法满足** |\n", mdList(rep.Summary.RequiresReencode))
	if len(rep.Summary.Insufficient) > 0 {
		fmt.Fprintf(&b, "| ⚪ insufficient | %s | 证据不足，无法判定 |\n", mdList(rep.Summary.Insufficient))
	}
	b.WriteString("\n")
	b.WriteString("> 判定原则：把每档关键帧的媒体 PTS 减去“首帧 PTS（最早合成时间）”，得到与时间基/起始标签无关的**内容相对时间**。")
	b.WriteString("若每个统一网格内容时刻都能匹配到闭合 IDR，则 GOP/IDR 编码结构合格；此时显示时刻若仍有偏差，")
	b.WriteString("只可能来自 `elst`/首帧 PTS/时间基等容器标签的一个恒定平移，可在不改动任何压缩样本的前提下修正。")
	b.WriteString("若统一内容时刻根本没有 IDR，或只有 CRA/BLA/非 IDR I（开放 GOP），任何封装/时间戳操作都无法“造出”IDR，必须重编码。")
	b.WriteString("B 帧会让 stss 的**解码样本序号**不等于 GOP×n，但 PTS 内容网格不受重排影响，因此判定以 PTS 为准、样本序号仅作参考。\n\n")

	b.WriteString("## 2. 统一关键帧约束（unified keyframe constraint）\n\n")
	b.WriteString("```json\n")
	if raw, err := PrettyJSON(c); err == nil {
		b.Write(raw)
	}
	b.WriteString("\n```\n\n")
	fmt.Fprintf(&b, "- 对齐时间线：**%s**（不是原始 DTS）\n", c.AlignTimeline)
	fmt.Fprintf(&b, "- 网格原点：%s；统一 GOP：%s", rs(c.OriginSeconds), rs(c.GOPSeconds))
	if c.GOPFrames > 0 {
		fmt.Fprintf(&b, " = %d 帧", c.GOPFrames)
	}
	if c.FPS.N > 0 {
		fmt.Fprintf(&b, " @ %s fps", c.FPS)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "- 容差：%s；要求闭合 IDR：%v；闭合 GOP：%v\n", rsm(c.ToleranceSeconds), c.RequireIDR, c.ClosedGOP)
	fmt.Fprintf(&b, "- 编辑列表策略：%s\n", c.EditListPolicy)
	fmt.Fprintf(&b, "- 首帧策略：%s\n\n", c.FirstFramePolicy)
	if len(c.GridKeyframeTimes) > 0 {
		var ts []string
		for _, t := range c.GridKeyframeTimes {
			ts = append(ts, rs(t))
		}
		fmt.Fprintf(&b, "- 网格关键帧时刻：%s\n\n", strings.Join(ts, ", "))
	}
	b.WriteString("编码器约束落地：\n\n")
	for _, h := range c.EncoderHints {
		fmt.Fprintf(&b, "- `%s`\n", h)
	}
	b.WriteString("\n")

	b.WriteString("## 3. GOP / IDR / 编辑列表 / 首帧时间横向比较\n\n")
	b.WriteString("| 档位 | 编码 | 帧率 | media TS | 首 DTS | 首 PTS(media) | 首显示 PTS(elst 后) | 编辑列表 | 关键帧(样本号) | IDR/非IDR/未知 | GOP 间隔 |\n")
	b.WriteString("| --- | --- | --- | ---: | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, f := range rep.Facts {
		idr, nonIDR, unk := idrCounts(f)
		var samples []string
		for _, k := range f.Keyframes {
			mark := ""
			if k.IDR != nil && !*k.IDR {
				mark = "✗"
			}
			samples = append(samples, fmt.Sprintf("%d%s", k.Sample, mark))
		}
		codec := dash(strings.ToUpper(f.Codec))
		if f.Width > 0 || f.Height > 0 {
			codec = fmt.Sprintf("%s %dx%d", codec, f.Width, f.Height)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s | **%s** | %s | `%s` | %d/%d/%d | %s |\n",
			f.ID, codec, dash(f.FPS.String()), f.MediaTimescale,
			rs(f.FirstDTS), rs(f.FirstPTS), rs(f.FirstDisplayPTS),
			escapePipes(elstSummary(f)), strings.Join(samples, ","),
			idr, nonIDR, unk, escapePipes(gopSummary(f)))
	}
	b.WriteString("\n带 ✗ 的样本号表示该同步点不是闭合 IDR（开放 GOP/CRA/非 IDR I）。\n\n")

	b.WriteString("## 4. 各档判定、偏差与处置\n\n")
	for _, v := range rep.Verdicts {
		fmt.Fprintf(&b, "### %s — `%s`\n\n", v.ID, v.Status)
		fmt.Fprintf(&b, "- 分类：**%s**；可不重编码满足：**%v**\n", statusCN(v.Status), v.RemuxPossible)
		fmt.Fprintf(&b, "- 显示时间对齐：%v；内容 PTS 网格覆盖：%v；解码帧序号恰好对齐：%v；网格点全为 IDR：%v；GOP 均匀：%v；帧率兼容：%v\n",
			v.DisplayAligned, v.ContentAligned, v.SamplesAligned, v.AllKeyframesIDR, v.GOPUniform, v.FPSCompatible)
		fmt.Fprintf(&b, "- 关键帧到网格最大显示偏差：%s\n", rsm(v.MaxDisplayError))
		if v.LabelOffsetSeconds != nil {
			fmt.Fprintf(&b, "- 恒定标签偏移 δ：%s（重封装需消除的量）\n", rsm(*v.LabelOffsetSeconds))
		}
		if v.MaxMediaCadenceError.N != 0 || v.MaxMediaCadenceError.D > 1 {
			fmt.Fprintf(&b, "- GOP 节奏相对统一值最大偏差：%s\n", rsm(v.MaxMediaCadenceError))
		}
		if len(v.MissingGridTimes) > 0 {
			var ms []string
			for _, t := range v.MissingGridTimes {
				ms = append(ms, rs(t))
			}
			fmt.Fprintf(&b, "- 缺失的网格关键帧时刻：%s\n", strings.Join(ms, ", "))
		}
		if len(v.NonIDRSamples) > 0 {
			fmt.Fprintf(&b, "- 非 IDR 网格样本：%v\n", v.NonIDRSamples)
		}
		b.WriteString("\n判定依据：\n\n")
		for _, r := range v.Reasons {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n处置动作：\n\n")
		for _, a := range v.Actions {
			fmt.Fprintf(&b, "- %s\n", a)
		}
		b.WriteString("\n")
	}

	b.WriteString("## 5. 为什么会黑屏 / 修复原理\n\n")
	b.WriteString("ABR/HLS/DASH 切换发生在 GOP/分段边界。播放器在新档必须从一个**闭合 IDR** 开始解码；")
	b.WriteString("若各档 IDR 对应的是不同内容帧（帧序号/GOP 不一致），或同一 IDR 经 `elst`、首帧 PTS、时间基换算后落在不同显示时刻，")
	b.WriteString("切换点在新档就不是随机接入点，解码器需要等到下一个 IDR，期间输出黑帧/冻结，表现为切换黑屏。\n\n")
	b.WriteString("- **时间基差异本身不是问题**：所有比较都换算为精确有理数秒。\n")
	b.WriteString("- **`elst` 与首帧 PTS 是常见“假错位”来源**：空编辑（priming/lead-in）、正 `media_time` 裁剪、")
	b.WriteString("负 CTS 补偿都会平移显示时间；只要帧序号网格与 IDR 性质正确，重写编辑列表/PTS 即可，不重编码。\n")
	b.WriteString("- **GOP 节奏/相位错误或开放 GOP 是“真错位”**：统一时刻没有 IDR，或只有依赖前文的 CRA/非 IDR I，")
	b.WriteString("封装层无法生成 IDR，只能按第 2 节约束重编码（固定帧 GOP、关闭场景切换插帧、闭合 IDR）。\n")
	return b.String()
}

func mdList(xs []string) string {
	if len(xs) == 0 {
		return "—"
	}
	return strings.Join(xs, ", ")
}

func escapePipes(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
