package analyzer

import (
	"fmt"
	"math"
)

func classify(report *Report) {
	source := report.Source
	scaled := report.Scaled
	c := report.Comparison

	sourceHasDetail := source.Spatial.EdgeSamples >= 20 ||
		source.Spatial.Gradient.RMS >= 0.5 || source.Spatial.HFNorm >= 0.05
	scaledHasDetail := scaled.Spatial.EdgeSamples >= 20 ||
		scaled.Spatial.Gradient.RMS >= 0.5 || scaled.Spatial.HFNorm >= 0.05
	sourceSoft := source.Spatial.EdgeWidthMedian >= 2.2 && source.Spatial.EdgeSamples >= 20
	edgeSoftening := source.Spatial.EdgeSamples >= 20 && scaled.Spatial.EdgeSamples >= 20 && c.EdgeWidthRatio >= 1.28
	hfSoftening := sourceHasDetail && scaledHasDetail && c.SharpnessRetention <= 0.72
	gradientSoftening := source.Spatial.Gradient.RMS >= 0.5 && scaled.Spatial.Gradient.RMS >= 0.5 && c.GradientRetention <= 0.72
	softening := edgeSoftening || hfSoftening || gradientSoftening

	sourceBlock := strongBlock(source.Block.Best)
	scaledBlock := strongBlock(scaled.Block.Best)
	sourceSameBlock := sameBlockResult(source.Block, scaled.Block.Best)
	newBlock := scaledBlock && (c.BlockExcessDelta >= 0.035 ||
		(validBlock(sourceSameBlock) && scaled.Block.Best.Confidence-sourceSameBlock.Confidence >= 0.18))

	sourceDelta := source.Temporal.DeltaNorm
	scaledDelta := scaled.Temporal.DeltaNorm
	sourceEdgeNoise := source.Temporal.EdgeNoiseNorm
	scaledEdgeNoise := scaled.Temporal.EdgeNoiseNorm
	flickerDelta := scaled.Frames >= 2 &&
		scaledDelta >= 0.01 &&
		(sourceDelta <= 0.005 || scaledDelta >= sourceDelta*1.8)
	flickerEdge := scaled.Frames >= 2 &&
		scaledEdgeNoise >= 0.01 &&
		(sourceEdgeNoise <= 0.005 || scaledEdgeNoise >= sourceEdgeNoise*1.8)
	newFlicker := flickerDelta || flickerEdge
	encodingEvidence := newBlock || newFlicker

	switch {
	case encodingEvidence:
		report.Verdict = VerdictEncoding
		report.Confidence = encodingConfidence(scaled.Block.Best, newBlock, newFlicker, c)
		report.Summary = "缩放后的序列出现了源帧没有的固定块网格或帧间不稳定；伪影更可能来自编码/传输链路，而不是显示器采样。"
	case (sourceSoft || sourceBlock) && softening:
		report.Verdict = VerdictSourceDisplay
		report.Confidence = clamp(0.58+softeningStrength(c)*0.3, 0, 0.9)
		if sourceSoft {
			report.Summary = "源帧边缘本身已经偏宽，缩放后又进一步变软且没有新增编码块/闪烁证据；源模糊与显示采样都有贡献。"
		} else {
			report.Summary = "源帧已存在块边界，缩放后又出现额外软化且没有新增编码块/闪烁证据；源侧编码与显示采样都有贡献。"
		}
	case sourceSoft:
		report.Verdict = VerdictSource
		report.Confidence = clamp(0.55+(source.Spatial.EdgeWidthMedian-2.2)*0.12, 0, 0.86)
		report.Summary = "源帧边缘本身已经偏宽，缩放后没有显著增加软化；软边主要来自源内容或源侧渲染/压缩。"
	case sourceBlock && !newBlock:
		report.Verdict = VerdictSource
		report.Confidence = clamp(0.66+source.Block.Best.Confidence*0.2, 0, 0.9)
		report.Summary = "源帧已存在稳定的块状边界，缩放后没有新增网格；伪影在进入显示采样前就已存在。"
	case softening:
		report.Verdict = VerdictDisplaySampling
		report.Confidence = clamp(0.58+softeningStrength(c)*0.32, 0, 0.92)
		report.Summary = "源帧边缘较锐，缩放后边缘按物理尺度明显变宽，且没有新增块网格或帧间闪烁；指向显示采样/缩放重建滤波。"
	default:
		report.Verdict = VerdictInconclusive
		report.Confidence = 0.35
		report.Summary = "没有检测到足以区分源、编码或显示采样的显著变化；请提供更紧的文字 ROI 和至少 3 帧静态画面。"
	}

	report.Confidence = clamp(report.Confidence, 0.05, 0.97)
	report.Evidence = buildEvidence(report, sourceSoft, sourceBlock, softening, newBlock, newFlicker)
}

func strongBlock(block BlockSizeStats) bool {
	return validBlock(block) && block.Confidence >= 0.55 && block.Excess >= 0.05 && block.ZScore >= 3.0
}

func validBlock(block BlockSizeStats) bool {
	return block.Size > 0 && block.Phase >= 0 && block.Axis != ""
}

func softeningStrength(c Comparison) float64 {
	edge := clamp((c.EdgeWidthRatio-1.28)/1.0, 0, 1)
	sharp := clamp((0.78-c.SharpnessRetention)/0.45, 0, 1)
	gradient := clamp((0.78-c.GradientRetention)/0.45, 0, 1)
	return clamp(0.45*edge+0.35*sharp+0.20*gradient, 0, 1)
}

func encodingConfidence(block BlockSizeStats, newBlock, newFlicker bool, c Comparison) float64 {
	score := 0.48
	if newBlock {
		score += 0.24 + block.Confidence*0.18 + clamp(c.BlockExcessDelta/0.15, 0, 1)*0.08
	}
	if newFlicker {
		score += 0.18
	}
	return clamp(score, 0, 0.95)
}

func buildEvidence(report *Report, sourceSoft, sourceBlock, softening, newBlock, newFlicker bool) []Evidence {
	source := report.Source
	scaled := report.Scaled
	c := report.Comparison
	evidence := []Evidence{
		{
			Code:  "edge_width",
			Value: c.EdgeWidthRatio,
			Text: fmt.Sprintf("边缘 10–90%% 过渡宽度：源 %.2f px，缩放后 %.2f px（归一化倍率 %.2f，比值 %.2f）。",
				source.Spatial.EdgeWidthMedian, scaled.Spatial.EdgeWidthMedian, c.Scale, c.EdgeWidthRatio),
		},
		{
			Code:  "sharpness_retention",
			Value: c.SharpnessRetention,
			Text:  fmt.Sprintf("按缩放倍率校正后，高频能量保留 %.2f，梯度 RMS 保留 %.2f。", c.SharpnessRetention, c.GradientRetention),
		},
		{
			Code:  "block_grid",
			Value: c.BlockExcessDelta,
			Text:  blockEvidenceText(source.Block, scaled.Block.Best, c.BlockExcessDelta),
		},
		{
			Code:  "temporal",
			Value: c.TemporalDeltaDiff,
			Text: fmt.Sprintf("相邻帧归一化差异：源 %.3f，缩放后 %.3f；强边缘时域噪声：源 %.3f，缩放后 %.3f。",
				source.Temporal.DeltaNorm, scaled.Temporal.DeltaNorm,
				source.Temporal.EdgeNoiseNorm, scaled.Temporal.EdgeNoiseNorm),
		},
	}
	if source.Frames < 2 || scaled.Frames < 2 {
		evidence = append(evidence, Evidence{
			Code: "single_frame",
			Text: "当前只有一帧，帧间稳定性无法验证；编码闪烁证据会缺失。建议至少 3 帧静态画面。",
		})
	}
	if sourceSoft {
		evidence = append(evidence, Evidence{
			Code:  "source_soft",
			Value: source.Spatial.EdgeWidthMedian,
			Text:  fmt.Sprintf("源帧边缘中位宽度 %.2f px，已超过锐利文字的经验阈值 2.2 px。", source.Spatial.EdgeWidthMedian),
		})
	}
	if sourceBlock {
		evidence = append(evidence, Evidence{
			Code:  "source_block_artifact",
			Value: source.Block.Best.Confidence,
			Text:  fmt.Sprintf("源帧已存在 %dpx %s 块边界（超额 %.1f%%，z=%.1f）。", source.Block.Best.Size, source.Block.Best.Axis, source.Block.Best.Excess*100, source.Block.Best.ZScore),
		})
	}
	if softening {
		evidence = append(evidence, Evidence{
			Code:  "added_softening",
			Value: c.EdgeWidthRatio,
			Text:  "缩放序列相对源帧出现显著边缘变宽或高频损失。",
		})
	}
	if newBlock {
		evidence = append(evidence, Evidence{
			Code:  "new_block_artifact",
			Value: scaled.Block.Best.Confidence,
			Text:  fmt.Sprintf("缩放帧新增/增强 %dpx %s 块边界，置信度 %.2f。", scaled.Block.Best.Size, scaled.Block.Best.Axis, scaled.Block.Best.Confidence),
		})
	}
	if newFlicker {
		evidence = append(evidence, Evidence{
			Code:  "new_temporal_instability",
			Value: c.TemporalDeltaDiff,
			Text:  "缩放帧在源帧稳定时出现更强的帧间变化，符合压缩/码率波动特征。",
		})
	}
	return evidence
}

func blockEvidenceText(sourceStats BlockStats, scaled BlockSizeStats, delta float64) string {
	if !validBlock(scaled) {
		return "没有检测到可用的块边界周期。"
	}
	sourcePart := "源帧无显著块网格"
	if source := sameBlockResult(sourceStats, scaled); validBlock(source) {
		sourcePart = fmt.Sprintf("源 %s/%dpx 超额 %.1f%%", source.Axis, source.Size, source.Excess*100)
	}
	return fmt.Sprintf("%s；缩放 %s/%dpx 超额 %.1f%%（同尺寸差 %+.1f 个百分点，z=%.1f）。",
		sourcePart, scaled.Axis, scaled.Size, scaled.Excess*100, delta*100, scaled.ZScore)
}

func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}
