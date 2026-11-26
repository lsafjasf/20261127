package flatfield

import (
	"fmt"
	"strings"
)

// SummaryText renders a compact human-readable attribution report.
func (t *Table) SummaryText() string {
	r := t.Report
	var b strings.Builder
	fmt.Fprintf(&b, "flatfield table: kind=%s grid=%dx%d source=%dx%d reference=%s\n",
		t.Kind, t.GridWidth, t.GridHeight, r.SourceWidth, r.SourceHeight, t.Reference)
	fmt.Fprintf(&b, "dominant cause: %s (confidence %.2f)\n", r.DominantCause, r.Confidence)
	fmt.Fprintf(&b, "identifiability: %s\n", r.Identifiability)
	if len(r.Contributions) > 0 {
		fmt.Fprintf(&b, "variance fractions: gradient=%.3f lens=%.3f panel_residual=%.3f\n",
			r.Contributions["gradient_fraction"], r.Contributions["lens_fraction"], r.Contributions["panel_residual_fraction"])
	}
	fmt.Fprintf(&b, "log RMS: observed=%.4f gradient=%.4f lens=%.4f panel=%.4f panel_residual=%.4f\n",
		r.Observed.LogRMS, r.Gradient.LogRMS, r.Lens.LogRMS, r.Panel.LogRMS, r.PanelResidual.LogRMS)
	fmt.Fprintf(&b, "gradient: slope=(%.5f,%.5f) magnitude=%.5f angle=%.1f r2=%.3f significant=%v\n",
		r.Gradient.LogSlopeX, r.Gradient.LogSlopeY, r.Gradient.MagnitudePerHeight,
		r.Gradient.AngleDegrees, r.Gradient.RSquared, r.Gradient.Significant)
	fmt.Fprintf(&b, "lens log coefficients r2/r4/r6: %.5f %.5f %.5f model=%s\n",
		r.Lens.Coefficients[0], r.Lens.Coefficients[1], r.Lens.Coefficients[2], r.Lens.Model)
	writeCorners(&b, "observed", r.Observed.CornerResponse)
	writeCorners(&b, "selected gain", cornerGains(t))
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "warning: %s\n", w)
	}
	return b.String()
}

func writeCorners(b *strings.Builder, name string, corners []CornerStats) {
	fmt.Fprintf(b, "%s corners TL/TR/BR/BL:", name)
	for _, c := range corners {
		fmt.Fprintf(b, " %.4f", c.Response)
	}
	fmt.Fprintln(b)
}

func cornerGains(t *Table) []CornerStats {
	out := make([]CornerStats, 4)
	coords := [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	for i, c := range coords {
		g := t.GainAt(c[0], c[1])
		out[i] = CornerStats{U: c[0], V: c[1], Response: g, Gain: g}
	}
	return out
}
