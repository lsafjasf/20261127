package hdrframe

import (
	"fmt"
	"strings"
)

// Text renders a human-readable Chinese audit report.
func (r *Report) Text() string {
	var b strings.Builder
	title := r.Name
	if title == "" {
		title = "HDR10+ 动态元数据逐帧核对"
	}
	fmt.Fprintf(&b, "%s\n", title)
	fmt.Fprintf(&b, "帧率=%s (%.6g fps) | 重复策略=%s | 缺元数据=%s | 跳号策略=%s\n",
		r.FPS, r.FPSFloat, r.Policy.Duplicate, r.Policy.MissingMetadata, r.Policy.SequenceGap)
	fmt.Fprintf(&b, "汇总: 目标帧=%d 匹配=%d 不匹配=%d 未直接验证=%d 缺精确记录=%d 重复键=%d(冲突%d) 跳号段=%d 场景不符=%d\n\n",
		r.Summary.TargetFrames, r.Summary.Matched, r.Summary.Mismatched, r.Summary.Unverified,
		r.Summary.MissingExactRecord, r.Summary.DuplicateKeys, r.Summary.DuplicateConflicts,
		r.Summary.SequenceGaps, r.Summary.SceneMismatches)

	if len(r.Duplicates) > 0 {
		b.WriteString("【重复记录】\n")
		for _, dup := range r.Duplicates {
			kind := "完全相同"
			if !dup.Identical {
				kind = "参数冲突"
			}
			fmt.Fprintf(&b, "- %s: 序列=%d 版本=%s 类型=%s 裁决=%s 选用=%s\n",
				dup.Key, dup.Sequence, strings.Join(dup.Versions, " | "), kind, dup.Resolution, dup.ChosenVersion)
		}
		b.WriteString("\n")
	}

	if len(r.SequenceGaps) > 0 {
		b.WriteString("【跳号】\n")
		for _, gap := range r.SequenceGaps {
			fmt.Fprintf(&b, "- 序列 %d -> %d，缺少 %d 个: %v；策略=%s\n",
				gap.AfterSequence, gap.BeforeSequence, gap.MissingCount, gap.Missing, gap.Action)
		}
		b.WriteString("\n")
	}

	b.WriteString("【目标帧裁决】\n")
	for _, frame := range r.Frames {
		fmt.Fprintf(&b, "\n帧 %d | PTS %.6fs | %s | 状态=%s | %s\n",
			frame.Frame, frame.PTSSeconds, frame.Timecode, frame.Status, frame.TimelineVerdict)
		if frame.Label != "" {
			fmt.Fprintf(&b, "  标记: %s\n", frame.Label)
		}
		fmt.Fprintf(&b, "  场景: 期望=%s", int64PtrString(frame.ExpectedSceneID))
		if frame.Expected != nil && frame.Expected.SceneID != nil {
			fmt.Fprintf(&b, " 源元数据=%s", int64PtrString(frame.Expected.SceneID))
		}
		if frame.Selected != nil && frame.Selected.SceneID != nil {
			fmt.Fprintf(&b, " 流转码元数据=%s", int64PtrString(frame.Selected.SceneID))
		}
		b.WriteString("\n")
		if frame.Expected != nil {
			fmt.Fprintf(&b, "  应有参数: %s seq=%d 生效=%.6fs(%s) 高光[peak=%.0f gain=%.2f] 色相[hue=%.1f sat=%.2f]\n",
				frame.Expected.Version, frame.Expected.Sequence, frame.Expected.EffectiveSeconds,
				frame.Expected.EffectiveTimecode, frame.Expected.Highlight.TargetPeakNits,
				frame.Expected.Highlight.Gain, frame.Expected.Color.HueShiftDegrees,
				frame.Expected.Color.Saturation)
		}
		if frame.Selected != nil {
			fmt.Fprintf(&b, "  时间线可选: %s seq=%d 生效=%.6fs(%s) 依据=%s 校验=%s 高光[peak=%.0f gain=%.2f] 色相[hue=%.1f sat=%.2f]\n",
				frame.Selected.Version, frame.Selected.Sequence, frame.Selected.EffectiveSeconds,
				frame.Selected.EffectiveTimecode, frame.Selected.SelectionBasis, shortChecksum(frame.Selected.Checksum),
				frame.Selected.Highlight.TargetPeakNits, frame.Selected.Highlight.Gain,
				frame.Selected.Color.HueShiftDegrees, frame.Selected.Color.Saturation)
		}
		if frame.Applied != nil {
			verify := "播放器日志证实"
			if !frame.Applied.Verified {
				verify = "策略推断(无直接日志)"
			}
			fmt.Fprintf(&b, "  实际应用: %s seq=%d 校验=%s | %s | %s\n",
				frame.Applied.Version, frame.Applied.Sequence, shortChecksum(frame.Applied.Checksum),
				verify, frame.Applied.EvidenceSource)
			fmt.Fprintf(&b, "    匹配时间线可选=%v 匹配应有参数=%v；原因: %s\n",
				frame.Applied.MatchesSelected, frame.Applied.MatchesExpected, frame.Applied.Reason)
			if frame.Applied.Evidence != "" {
				fmt.Fprintf(&b, "    证据: %s\n", frame.Applied.Evidence)
			}
		} else {
			b.WriteString("  实际应用: 无法确定（没有播放器日志，也没有可保持的上一版）\n")
		}
		for _, diff := range frame.ParameterDiffs {
			fmt.Fprintf(&b, "  参数差异: %s 应有=%.3f 实际=%.3f 差值=%+.3f；%s\n",
				diff.Label, diff.Expected, diff.Applied, diff.Delta, diff.Effect)
		}
		for _, issue := range frame.Issues {
			fmt.Fprintf(&b, "  ! [%s/%s] %s\n", issue.Severity, issue.Code, issue.Message)
		}
	}

	if len(r.Diagnostics) > 0 {
		b.WriteString("\n【输入诊断】\n")
		for _, issue := range r.Diagnostics {
			fmt.Fprintf(&b, "- [%s/%s] %s\n", issue.Severity, issue.Code, issue.Message)
		}
	}
	return b.String()
}

func shortChecksum(checksum string) string {
	if checksum == "" {
		return "-"
	}
	if len(checksum) > 24 {
		return checksum[:24]
	}
	return checksum
}
