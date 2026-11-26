package hdrswitch

import (
	"fmt"
	"strings"
)

func RenderText(report *Report) string {
	var out strings.Builder

	if risk := report.GOPRisk; risk != nil && (risk.AtRisk || !risk.Safe) {
		fmt.Fprintln(&out, "GOP 风险（最高优先级）")
		fmt.Fprintf(&out, "  状态        : at_risk=%v safe=%v\n", risk.AtRisk, risk.Safe)
		fmt.Fprintf(&out, "  切点 AU/PTS : %d / %s\n", risk.CutAUIndex, risk.CutPTS.SecondsText(6))
		if risk.GOPStartAUIndex != nil && risk.GOPStartPTS != nil {
			fmt.Fprintf(&out, "  GOP 起点    : AU %d / %s，切点偏移 %d 个 AU\n",
				*risk.GOPStartAUIndex, risk.GOPStartPTS.SecondsText(6), risk.PositionFromGOPStart)
		}
		if risk.PreviousRandomAccess != nil {
			boundary := risk.PreviousRandomAccess
			fmt.Fprintf(&out, "  前一接入点  : AU %d / %s / %s\n", boundary.AUIndex, boundary.PTS.SecondsText(6), boundary.Type)
		}
		if risk.NextRandomAccess != nil {
			boundary := risk.NextRandomAccess
			fmt.Fprintf(&out, "  下一接入点  : AU %d / %s / %s\n", boundary.AUIndex, boundary.PTS.SecondsText(6), boundary.Type)
		}
		fmt.Fprintf(&out, "  原因        : %s\n", risk.Reason)
		fmt.Fprintf(&out, "  必须动作    : %s\n\n", risk.RequiredAction)
	}

	fmt.Fprintf(&out, "结论: %s\n", report.Classification)
	fmt.Fprintf(&out, "%s\n\n", report.Summary)
	fmt.Fprintf(&out, "切点: %s；AU 对齐: %s", report.SwitchAt.SecondsText(6), report.CutAlignment)
	if report.CutAUIndex != nil {
		fmt.Fprintf(&out, "；切点 AU: %d", *report.CutAUIndex)
	}
	fmt.Fprintln(&out)
	if risk := report.GOPRisk; risk != nil && (risk.Safe || !risk.AtRisk) {
		fmt.Fprintf(&out, "GOP 边界: at_risk=%v safe=%v；%s\n", risk.AtRisk, risk.Safe, risk.Reason)
	}

	renderSegmentText(&out, "切点前合法范围", report.Before)
	renderSegmentText(&out, "切点后合法范围", report.After)

	if report.Player != nil {
		fmt.Fprintln(&out, "\n播放器/全局元数据:")
		fmt.Fprintf(&out, "  沿用首帧     : %v；失配 AU 数: %d\n", report.Player.StickyFirstFrame, report.Player.MismatchAUCount)
		if report.Player.FirstMismatchAU != nil {
			fmt.Fprintf(&out, "  首个失配 AU  : %d；字段: %v\n", *report.Player.FirstMismatchAU, report.Player.StaleFields)
		}
	}

	fmt.Fprintln(&out, "\nAU 元数据变化/失配:")
	fmt.Fprintln(&out, "  AU     PTS(s)    cut  IDR  changed                         player-stale")
	printed := 0
	for _, state := range report.AccessUnits {
		if !state.MetadataChanged && !state.PlayerStale && !state.IsCutAU {
			continue
		}
		if printed >= 20 {
			fmt.Fprintln(&out, "  ...（其余见 JSON access_units）")
			break
		}
		changed := strings.Join(state.ChangedFields, ",")
		if changed == "" {
			changed = "-"
		}
		stale := "-"
		if state.PlayerStale {
			stale = strings.Join(state.StaleFields, ",")
		}
		cut := "-"
		if state.IsCutAU {
			cut = "CUT"
		}
		idr := "-"
		if state.IDR {
			idr = "IDR"
		}
		fmt.Fprintf(&out, "  %-6d %-9s %-4s %-4s %-31s %s\n",
			state.Index, state.PTS.Text(6), cut, idr, changed, stale)
		printed++
	}

	fmt.Fprintln(&out, "\n核对结果:")
	for _, finding := range report.Findings {
		fmt.Fprintf(&out, "  [%s] %s: %s\n", finding.Severity, finding.Code, finding.Message)
	}
	return out.String()
}

func renderSegmentText(out *strings.Builder, title string, segment *SegmentReport) {
	if segment == nil {
		return
	}
	envelope := segment.LegalRange
	fmt.Fprintf(out, "\n%s (%s)\n", title, segment.ID)
	end := "unknown"
	if segment.EndPTS != nil {
		end = segment.EndPTS.SecondsText(6)
	}
	fmt.Fprintf(out, "  区间          : [%s, %s)；AU %d..%d，共 %d，stable=%v\n",
		segment.StartPTS.SecondsText(6), end, segment.FirstAUIndex, segment.LastAUIndex, segment.AUCount, segment.Stable)
	fmt.Fprintf(out, "  转换函数      : %s (H.273 code %d)\n", envelope.Transfer, envelope.H273TransferCode)
	fmt.Fprintf(out, "  色域          : %s (H.273 code %d)\n", envelope.Gamut.Primaries, envelope.Gamut.PrimariesCode)
	fmt.Fprintf(out, "  量化码段      : %d-bit %s；Y/R/G/B %d..%d；Cb/Cr %d..%d\n",
		envelope.ComponentCodes.BitDepth, envelope.ComponentCodes.Quantization,
		envelope.ComponentCodes.Luma.Min, envelope.ComponentCodes.Luma.Max,
		envelope.ComponentCodes.Chroma.Min, envelope.ComponentCodes.Chroma.Max)
	fmt.Fprintf(out, "  非线性信号    : E' ∈ [%.0f, %.0f]\n", envelope.NonlinearSignal.Min, envelope.NonlinearSignal.Max)

	if envelope.Transfer == TransferPQ && envelope.RepresentationLuminance != nil {
		fmt.Fprintf(out, "  PQ 绝对亮度   : %.0f..%.0f nits（传递函数表示上限；不代表 MaxCLL）\n",
			envelope.RepresentationLuminance.Min, envelope.RepresentationLuminance.Max)
	}
	if envelope.SceneLinearSignal != nil {
		fmt.Fprintf(out, "  HLG 场景线性  : E ∈ [%.0f, %.0f]；无固定绝对 nit 上限\n",
			envelope.SceneLinearSignal.Min, envelope.SceneLinearSignal.Max)
	}
	if envelope.ReferenceLuminance != nil {
		fmt.Fprintf(out, "  声明参考峰值  : %.4g..%.4g nits；%s\n",
			envelope.ReferenceLuminance.Min, envelope.ReferenceLuminance.Max, envelope.Peak.CeilingBasis)
	} else {
		fmt.Fprintf(out, "  峰值          : AU/SEI 未给出可用声明；%s\n", envelope.Peak.CeilingBasis)
	}
	fmt.Fprintf(out, "  MaxCLL/MaxFALL: %s / %s nits；Mastering max/min: %s / %s\n",
		ratText(envelope.Peak.MaxCLL), ratText(envelope.Peak.MaxFALL),
		ratText(envelope.Peak.MasteringMaxNits), ratText(envelope.Peak.MasteringMinNits))
	fmt.Fprintf(out, "  CIE/SEI 坐标  : xy ∈ [0,1]；SEI 整数 %d..%d（单位 1/50000）\n",
		envelope.Gamut.SEICoordinates.Min, envelope.Gamut.SEICoordinates.Max)
	fmt.Fprintf(out, "  白点          : (%.4f, %.4f)\n", envelope.Gamut.White.X, envelope.Gamut.White.Y)
}

func ratText(value *Rational) string {
	if value == nil {
		return "unset"
	}
	return value.Text(3)
}
