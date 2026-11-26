package check

import "strings"

func buildVerdict(frame FrameRef, items []indexedObservation, recovery *Recovery, impactedBy []string, graphProvided bool) Verdict {
	verdict := Verdict{
		FrameKey:    frame.Key,
		LayerStatus: layerStatuses(items),
		Recovery:    recovery,
		ImpactedBy:  impactedBy,
	}

	var failures, passes []Observation
	trustedPixelPass := false
	for _, item := range items {
		obs := item.value
		switch obs.Status {
		case StatusFail:
			failures = append(failures, obs)
		case StatusPass:
			passes = append(passes, obs)
			if obs.Layer == LayerPixels && obs.TrustedReference {
				trustedPixelPass = true
			}
		}
	}

	pixelFail := verdict.LayerStatus[LayerPixels] == StatusFail
	decodeFail := verdict.LayerStatus[LayerDecode] == StatusFail
	bitstreamFail := verdict.LayerStatus[LayerBitstream] == StatusFail
	containerFail := verdict.LayerStatus[LayerContainer] == StatusFail
	containerAnomaly := containerFail || verdict.LayerStatus[LayerContainer] == StatusWarn
	crcPass := hasPass(passes, LayerBitstream)
	essenceFailure := bitstreamFail || pixelFail || decodeFail
	if len(impactedBy) > 0 {
		verdict.Warnings = append(verdict.Warnings,
			"参考链上的失败帧 "+strings.Join(impactedBy, ", ")+" 可能传播错误；本帧即使 CRC 通过，也不能仅凭 CRC 判定像素正确。")
	}
	if essenceFailure && !graphProvided {
		verdict.Warnings = append(verdict.Warnings, "未提供帧参考关系，无法排除该异常向后续 inter 帧传播；需结合参考图和下一个 IDR 边界评估。")
	}

	if bitstreamFail && pixelFail {
		verdict.Reasons = append(verdict.Reasons, "码流 CRC 失败说明编码字节或校验值不一致；像素失败独立证明解码结果与可信基准不同。")
		verdict.Reasons = append(verdict.Reasons, "两类失败同时出现不能证明物理损坏的先后；因果定位还需解码序、包字节和参考帧依赖。")
	}
	if containerAnomaly && !bitstreamFail && !pixelFail && !decodeFail {
		verdict.Reasons = append(verdict.Reasons, "异常局限在容器元数据/解封装校验；码流、解码和像素证据没有否定 essence 内容。")
	}
	if crcPass && !trustedPixelPass {
		verdict.Warnings = append(verdict.Warnings, "没有可信像素哈希或原始帧逐像素比对时，CRC 通过不能证明像素相同。")
	}
	if crcPass && pixelFail {
		verdict.Warnings = append(verdict.Warnings, "CRC 通过但像素校验失败；在核查覆盖范围、期望值和解码器行为前，必须按内容错误处理。")
	}

	if recovery != nil && recovery.GoodPixels {
		verdict.Reconstructable = ReconstructYes
		verdict.Mode = "copy_independent_known_good_pixels"
		verdict.Reasons = append(verdict.Reasons, "存在独立的已知良好原始帧或像素基准。")
		return verdict
	}

	if trustedPixelPass {
		verdict.Reconstructable = ReconstructYes
		verdict.Mode = "preserve_already_verified_pixels"
		verdict.Reasons = append(verdict.Reasons, "像素已匹配可信基准，因此当前解码帧已经像素一致；应保留原始像素或直接重封装。")
		if containerAnomaly {
			verdict.Reasons = append(verdict.Reasons, "修复容器元数据或无损重封装即可保留这些像素，不要重新编码。")
		}
		if bitstreamFail {
			verdict.Warnings = append(verdict.Warnings, "即使存储码流/CRC 工件异常，像素仍可被证明正确；应保留原始像素，但不能声称码流位一致。")
		}
		if len(impactedBy) > 0 {
			verdict.Warnings = append(verdict.Warnings, "可信像素校验已证明本帧实际输出正确，可覆盖参考传播风险。")
		}
		return verdict
	}

	if recovery != nil && recovery.GoodPacket {
		if recovery.BitExactDecoder {
			verdict.Reconstructable = ReconstructYes
			verdict.Mode = "redecode_known_good_packet_with_bit_exact_decoder"
			verdict.Reasons = append(verdict.Reasons, "存在独立的已知良好码包，且可使用位精确解码器重解。")
			return verdict
		}
		verdict.Reconstructable = ReconstructUnknown
		verdict.Mode = "good_packet_needs_decoder_attestation"
		verdict.Blockers = append(verdict.Blockers, "虽有已知良好码包，但尚未证明解码器位精确或与参考解码器一致。")
		return verdict
	}

	if pixelFail {
		verdict.Reconstructable = ReconstructNo
		verdict.Mode = "current_pixels_failed_trusted_verification"
		verdict.Blockers = append(verdict.Blockers, "当前解码像素与可信基准不匹配。")
		verdict.Blockers = append(verdict.Blockers, "失败的 CRC/校验值不能反推出原始像素；必须有独立好码包、原始帧、冗余副本或原始源。")
		return verdict
	}

	if decodeFail {
		verdict.Reconstructable = ReconstructNo
		verdict.Mode = "decode_failed_without_trusted_output"
		verdict.Blockers = append(verdict.Blockers, "解码器失败，且没有可信解码输出或独立恢复源。")
		return verdict
	}

	verdict.Reconstructable = ReconstructUnknown
	verdict.Mode = "needs_trusted_pixel_or_source_evidence"
	verdict.Blockers = append(verdict.Blockers, "缺少可信像素匹配，也缺少独立已知良好码包/原始帧。")
	if len(impactedBy) > 0 {
		verdict.Blockers = append(verdict.Blockers, "本帧位于失败帧的参考传播下游，且没有可信像素结果。")
	}
	if crcPass {
		verdict.Blockers = append(verdict.Blockers, "CRC 通过不足以证明像素完全相同。")
	}
	return verdict
}

func layerStatuses(items []indexedObservation) map[Layer]Status {
	statuses := map[Layer]Status{
		LayerContainer: StatusUnknown,
		LayerBitstream: StatusUnknown,
		LayerDecode:    StatusUnknown,
		LayerPixels:    StatusUnknown,
	}
	byLayer := make(map[Layer][]Status)
	for _, item := range items {
		byLayer[item.value.Layer] = append(byLayer[item.value.Layer], item.value.Status)
	}
	for layer, values := range byLayer {
		statuses[layer] = aggregateStatus(values)
	}
	return statuses
}

func aggregateStatus(values []Status) Status {
	hasFail := false
	hasWarn := false
	hasUnknown := false
	hasPass := false
	for _, value := range values {
		switch value {
		case StatusFail:
			hasFail = true
		case StatusWarn:
			hasWarn = true
		case StatusUnknown:
			hasUnknown = true
		case StatusPass:
			hasPass = true
		}
	}
	if hasFail {
		return StatusFail
	}
	if hasWarn {
		return StatusWarn
	}
	if hasUnknown {
		return StatusUnknown
	}
	if hasPass {
		return StatusPass
	}
	return StatusUnknown
}

func hasPass(observations []Observation, layer Layer) bool {
	for _, obs := range observations {
		if obs.Layer == layer && obs.Status == StatusPass {
			return true
		}
	}
	return false
}
