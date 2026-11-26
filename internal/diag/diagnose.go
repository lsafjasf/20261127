package diag

import (
	"fmt"
	"time"

	"seekdiag/internal/clock"
	"seekdiag/internal/mp4"
	"seekdiag/internal/seek"
)

const resyncWarning = "Periodic resync is not a fix. Diagnose and correct the first layer whose error diverges; only an explicit user seek may establish a new anchor."

func Analyze(f *mp4.File, target time.Duration, observations *ObservationSet, opts Options) *Result {
	seekMS := float64(target) / float64(time.Millisecond)
	result := &Result{
		File: f.Path, SeekMS: seekMS, ResyncWarning: resyncWarning,
	}
	plan, planErr := seek.At(f, target)
	if planErr != nil {
		result.Findings = append(result.Findings, Finding{
			Layer: VerdictContainer, Code: "seek_mapping_failed", Severity: "error",
			Message: planErr.Error(),
		})
	} else {
		result.SeekPlan = plan
		anchor, anchorErr := clock.AnchorFromSeek(plan)
		if anchorErr == nil {
			result.SeekAnchor = &anchor
		}
	}
	views, findings := inspectTracks(f)
	result.Tracks = views
	result.Findings = append(result.Findings, findings...)

	var obsVerdict string
	if observations != nil {
		report, obsFindings, verdict := analyzeObservations(f.Tracks, observations, opts)
		result.Observations = report
		result.Findings = append(result.Findings, obsFindings...)
		obsVerdict = verdict
	}

	hardContainer := hasError(result.Findings, VerdictContainer)
	switch {
	case hardContainer:
		result.Verdict = VerdictContainer
		result.Confidence = "high"
		result.Summary = "容器/样本表或编辑列表的时间映射本身不一致，seek 落点不能用该时间轴恢复。"
	case observations == nil:
		result.Verdict = VerdictInconclusive
		result.Confidence = "medium"
		result.Summary = "容器、样本位置和编辑列表未发现足以解释渐进延迟的硬错误；仅凭容器不能区分解码器与播放时钟。请采集 seek 后探针。"
	case obsVerdict == VerdictContainer || obsVerdict == VerdictDecoder || obsVerdict == VerdictClock:
		result.Verdict = obsVerdict
		result.Confidence = confidence(result.Findings, observations)
		result.Summary = observationSummary(obsVerdict)
	case obsVerdict == VerdictOK:
		result.Verdict = VerdictOK
		result.Confidence = "medium"
		result.Summary = "现有探针中容器 PTS、解码 PTS 与播放时钟未出现超过阈值的渐进偏差；当前数据未复现故障。"
	default:
		result.Verdict = VerdictInconclusive
		result.Confidence = "low"
		result.Summary = "探针不足（至少需要 seek 后两个样本，建议覆盖数秒以上），无法确定漂移层。"
	}
	return result
}

func hasError(findings []Finding, layer string) bool {
	for _, f := range findings {
		if f.Layer == layer && f.Severity == "error" {
			return true
		}
	}
	return false
}

func confidence(findings []Finding, obs *ObservationSet) string {
	for _, d := range findings {
		if d.Code == "post_seek_drift" {
			if len(obs.After) >= 3 {
				return "high"
			}
			return "medium"
		}
	}
	return "medium"
}

func observationSummary(layer string) string {
	switch layer {
	case VerdictContainer:
		return "seek 后观测到的音频 PTS 偏离独立解析出的样本/编辑列表时间轴，故障在容器时间映射。"
	case VerdictDecoder:
		return "容器包 PTS 正确，但解码输出的音频 PTS 相对包 PTS 渐进偏移，故障在解码/重排路径。"
	case VerdictClock:
		return "容器与解码 PTS 一致，而播放时钟相对解码 PTS 渐进漂移，故障在播放时钟/音频主时钟。"
	default:
		return fmt.Sprintf("诊断结果: %s", layer)
	}
}
