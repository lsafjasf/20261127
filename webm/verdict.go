package webm

import "alphacheck/preview"

func buildVerdict(findings []Finding) Verdict {
	v := Verdict{Status: "pass", ErrorCounts: map[string]int{}}
	firstErrorClass := ""
	hasError := false
	for _, f := range findings {
		if f.Severity != "error" {
			continue
		}
		hasError = true
		v.ErrorCounts[f.Class]++
		if firstErrorClass == "" {
			firstErrorClass = f.Class
		}
	}
	if !hasError {
		if len(findings) > 0 {
			v.Status = "warning"
		}
		v.ResponsibilityZH = "未发现阻断性错误"
		return v
	}
	v.Status = "fail"
	v.PrimaryClass = firstErrorClass

	_, mux := v.ErrorCounts["muxing_error"]
	_, enc := v.ErrorCounts["encoding_error"]
	_, comp := v.ErrorCounts["composition_error"]
	switch {
	case (mux && enc) || (mux && comp) || (enc && comp):
		v.PrimaryClass = "mixed_errors"
		v.ResponsibilityZH = "存在跨层错误；按 findings 中最早错误定位，不能只凭 codec 名称归因"
	case enc:
		v.ResponsibilityZH = "编码/码流/导出层错误：容器字段可能正确，但 VP8/VP9、Alpha 码流或解码后的 Alpha 像素无效"
	case comp:
		v.ResponsibilityZH = "画面合成层错误：黑白底预览或 Alpha 合成结果异常"
	case mux:
		v.ResponsibilityZH = "合成/封装层错误：AlphaMode、裁剪合同或帧时间与 Block 数据不一致"
	default:
		v.ResponsibilityZH = "存在未分类错误"
	}
	return v
}

// AttachPreview merges decoded black/white preview metrics into the verdict.
func (r *Report) AttachPreview(result preview.Result) {
	r.Preview = &result
	if result.Assessment.Status != "fail" && result.Assessment.Status != "warning" {
		return
	}
	severity := "warning"
	if result.Assessment.Status == "fail" {
		severity = "error"
	}
	class := result.Assessment.Class
	layer := "pixels"
	if class == "" {
		class = "composition_error"
	}
	if class == "encoding_error" {
		layer = "pixels"
	}
	for _, code := range result.Assessment.Codes {
		r.Findings = append(r.Findings, Finding{
			Severity: severity, Layer: layer, Class: class, Code: "preview_" + code,
			Message: result.Assessment.ResponsibilityZH, Evidence: result.Assessment.Message,
		})
	}
	r.Verdict = buildVerdict(r.Findings)
}
