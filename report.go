package flatfield

import (
	"errors"
	"math"
)

var errFitFailed = errors.New("flatfield: weighted least-squares fit is rank deficient")

func buildReport(tab *Table, samples []cellSample, z []float64, weights []float64, beta [6]float64, cfg Options, independentLens bool) {
	gw, gh := tab.GridWidth, tab.GridHeight
	tab.Report.Observed = componentStats(tab.ObservedResponse, gw, gh)
	tab.Report.Gradient.ComponentStats = componentStats(tab.GradientResponse, gw, gh)
	tab.Report.Lens.ComponentStats = componentStats(tab.LensResponse, gw, gh)
	tab.Report.Panel = componentStats(tab.PanelResponse, gw, gh)
	tab.Report.PanelResidual = componentStats(tab.PanelResidualResponse, gw, gh)

	slopeMag := math.Hypot(beta[1], beta[2])
	tab.Report.Gradient.LogSlopeX = beta[1]
	tab.Report.Gradient.LogSlopeY = beta[2]
	tab.Report.Gradient.MagnitudePerHeight = slopeMag
	if slopeMag > 0 {
		tab.Report.Gradient.AngleDegrees = math.Atan2(beta[2], beta[1]) * 180 / math.Pi
	}
	tab.Report.Gradient.Significant = beta[1] != 0 || beta[2] != 0
	tab.Report.Gradient.RSquared = weightedGradientRSquared(samples, z, beta, weights)
	if tab.Report.Gradient.Significant {
		tab.Report.Gradient.Interpretation = "affine_component_excluded_from_default_device_gain; may be uneven target illumination or asymmetric panel response"
	} else {
		tab.Report.Gradient.Interpretation = "no affine gradient above significance threshold"
	}

	tab.Report.Lens.Coefficients = [3]float64{beta[3], beta[4], beta[5]}
	if independentLens {
		tab.Report.Lens.Model = "independent reference table"
	} else {
		tab.Report.Lens.Model = "weighted log radial polynomial a*r^2+b*r^4+c*r^6"
	}

	gv := tab.Report.Gradient.LogRMS * tab.Report.Gradient.LogRMS
	lv := tab.Report.Lens.LogRMS * tab.Report.Lens.LogRMS
	pv := tab.Report.PanelResidual.LogRMS * tab.Report.PanelResidual.LogRMS
	total := gv + lv + pv
	tab.Report.Contributions = map[string]float64{
		"gradient_variance":       gv,
		"lens_variance":           lv,
		"panel_residual_variance": pv,
		"panel_variance":          tab.Report.Panel.LogRMS * tab.Report.Panel.LogRMS,
	}
	if total > 0 {
		tab.Report.Contributions["gradient_fraction"] = gv / total
		tab.Report.Contributions["lens_fraction"] = lv / total
		tab.Report.Contributions["panel_residual_fraction"] = pv / total
		tab.Report.Contributions["panel_fraction"] = pv / total
	}

	maxSD := math.Max(tab.Report.Gradient.LogRMS, math.Max(tab.Report.Lens.LogRMS, tab.Report.PanelResidual.LogRMS))
	switch {
	case maxSD < 0.008:
		tab.Report.DominantCause = "uniform"
	case total > 0 && lv/total >= 0.45 && lv >= gv && lv >= pv:
		tab.Report.DominantCause = "lens_vignetting"
	case total > 0 && pv/total >= 0.45 && pv >= gv && pv >= lv:
		tab.Report.DominantCause = "panel_nonuniformity"
	case total > 0 && gv/total >= 0.45 && gv >= lv && gv >= pv:
		tab.Report.DominantCause = "scene_or_illumination_gradient"
	default:
		tab.Report.DominantCause = "mixed"
	}
	if tab.Report.DominantCause == "mixed" && tab.Report.Panel.LogRMS > 0.02 && lv < 0.0001 && gv < 0.0001 {
		tab.Report.DominantCause = "panel_nonuniformity"
	}

	if independentLens {
		tab.Report.Identifiability = "multiplicative_model;lens_independently_referenced;remaining_affine_field_is_gradient_or_asymmetric_panel"
		tab.Report.Confidence = 0.9
	} else {
		tab.Report.Identifiability = "single_flat_field_cannot_uniquely_separate_coincident_radial_panel_falloff_and_lens_vignetting;radial_term_assigned_to_lens,residual_to_panel"
		tab.Report.Confidence = 0.66
		if tab.Report.DominantCause == "lens_vignetting" || tab.Report.DominantCause == "panel_nonuniformity" {
			tab.Report.Confidence = 0.72
		}
		tab.Report.Warnings = append(tab.Report.Warnings,
			"supply an independent lens table or uniform-target lens flat for definitive lens/panel attribution")
	}
	tab.Report.Warnings = append(tab.Report.Warnings,
		"real scene gradients are image content; multiplicative table correction divides out device response and preserves them")
	if tab.Report.Gradient.Significant {
		tab.Report.Warnings = append(tab.Report.Warnings,
			"significant affine gradient is included in default device/panel fields; use device_no_gradient or panel_residual only if it is external target illumination rather than panel response")
	}
}

func componentStats(resp []float64, gw, gh int) ComponentStats {
	st := ComponentStats{MinResponse: math.Inf(1), MaxResponse: math.Inf(-1)}
	logs := make([]float64, len(resp))
	weight := 1.0 / float64(len(resp))
	meanLog, meanResp := 0.0, 0.0
	for i, r := range resp {
		logs[i] = math.Log(r)
		meanLog += logs[i] * weight
		meanResp += r * weight
		st.MinResponse = math.Min(st.MinResponse, r)
		st.MaxResponse = math.Max(st.MaxResponse, r)
	}
	variance, relVariance := 0.0, 0.0
	for i, r := range resp {
		d := logs[i] - meanLog
		variance += d * d * weight
		if meanResp > 0 {
			rd := r/meanResp - 1
			relVariance += rd * rd * weight
		}
	}
	st.LogRMS = math.Sqrt(variance)
	st.RelativeRMS = math.Sqrt(relVariance)

	corners := [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	st.CornerResponse = make([]CornerStats, 4)
	for i, c := range corners {
		r := sampleGridArray(resp, gw, gh, c[0], c[1])
		st.CornerResponse[i] = CornerStats{U: c[0], V: c[1], Response: r, Gain: 1 / r}
	}
	return st
}

func weightedGradientRSquared(samples []cellSample, z []float64, beta [6]float64, weights []float64) float64 {
	wSum, zMean, gMean := 0.0, 0.0, 0.0
	gs := make([]float64, len(samples))
	for i, s := range samples {
		w := weights[i]
		wSum += w
		gs[i] = beta[1]*s.dx + beta[2]*s.dy
		zMean += z[i] * w
		gMean += gs[i] * w
	}
	if wSum <= 0 {
		return 0
	}
	zMean /= wSum
	gMean /= wSum
	cov, vz, vg := 0.0, 0.0, 0.0
	for i := range samples {
		zd := z[i] - zMean
		gd := gs[i] - gMean
		cov += weights[i] * zd * gd
		vz += weights[i] * zd * zd
		vg += weights[i] * gd * gd
	}
	if vz <= 0 || vg <= 0 {
		return 0
	}
	r2 := (cov * cov) / (vz * vg)
	return clampFloat(r2, 0, 1)
}
