package preview

// Assessment interprets black/white proof metrics.
type Assessment struct {
	Status           string   `json:"status"`
	Class            string   `json:"class,omitempty"`
	Codes            []string `json:"codes,omitempty"`
	ResponsibilityZH string   `json:"responsibility_zh,omitempty"`
	Message          string   `json:"message,omitempty"`
}

// Result combines numeric preview statistics and their interpretation.
type Result struct {
	Stats
	Assessment Assessment `json:"assessment"`
}

// Evaluate builds a Result.
func Evaluate(stats Stats, requireVariation bool) Result {
	return Result{Stats: stats, Assessment: Assess(stats, requireVariation)}
}

// Assess decides whether decoded alpha behaves like a useful alpha channel.
// requireVariation should be true when the source is known to contain a
// transparent logo/overlay. A constant-alpha track is structurally valid but
// cannot provide transparency.
func Assess(stats Stats, requireVariation bool) Assessment {
	switch {
	case stats.Width <= 0 || stats.Height <= 0:
		return Assessment{Status: "fail", Class: "encoding_error", Codes: []string{"empty_preview"}, ResponsibilityZH: "编码/解码层错误：没有可评估的预览帧"}
	case stats.AlphaMin == 255 && stats.AlphaMax == 255:
		status := "warning"
		if requireVariation {
			status = "fail"
		}
		return Assessment{
			Status: status, Class: "encoding_error", Codes: []string{"alpha_constant_opaque"},
			ResponsibilityZH: "编码/导出层错误：Alpha 载荷存在，但解码后的 Alpha 平面全为 255；黑白底必然相同",
			Message:          "if transparency is expected, inspect the RGBA->yuva conversion and alpha encoder input",
		}
	case stats.AlphaMax == 0:
		return Assessment{
			Status: "fail", Class: "encoding_error", Codes: []string{"alpha_constant_transparent"},
			ResponsibilityZH: "编码/导出层错误：Alpha 平面全为 0",
		}
	case stats.BlackWhiteMeanDiff < 1.0:
		return Assessment{
			Status: "fail", Class: "encoding_error", Codes: []string{"identical_black_white_proofs"},
			ResponsibilityZH: "编码/合成层异常：存在中间 Alpha 值，但黑白底预览没有差异",
		}
	case stats.WhiteHaloLumaDistance <= -12:
		return Assessment{
			Status: "fail", Class: "composition_error", Codes: []string{"dark_white_halo_double_premultiplication"},
			ResponsibilityZH: "画面合成错误：白底下边缘偏黑，符合预乘色被按 straight alpha 再乘一次；Alpha 编码本身未必损坏",
		}
	case stats.WhiteHaloLumaDistance >= 12:
		return Assessment{
			Status: "fail", Class: "composition_error", Codes: []string{"bright_edge_halo_alpha_convention_mismatch"},
			ResponsibilityZH: "画面合成错误：边缘异常偏亮，检查 straight/premultiplied alpha 约定",
		}
	default:
		return Assessment{
			Status: "pass", Class: "composition_ok", Codes: []string{"black_white_proofs_separate"},
			ResponsibilityZH: "黑白底按 Alpha 正常分离；若边缘仅在一种底色下发脏，再查 straight/premultiplied 合成",
		}
	}
}
