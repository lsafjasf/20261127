package observermap

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// WriteJSON renders any report value as indented JSON.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// WriteDemoText renders the human-readable showcase tables.
func WriteDemoText(w io.Writer, r *DemoReport) {
	fmt.Fprintln(w, "CIE 2° (1931) ↔ 10° (1964) observer empirical mapping demo")
	fmt.Fprintln(w, repeat("=", 72))
	fmt.Fprintln(w, "⚠ "+r.Disclaimer)
	fmt.Fprintln(w)

	WriteCertaintyText(w, r.Profile)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Empirical 3x3 map XYZ_10_hat = M · XYZ_2  (D65 fit)")
	fmt.Fprintln(w, repeat("-", 72))
	writeMatrix(w, r.MapD65.Matrix)
	q := r.MapD65.Quality
	fmt.Fprintf(w, "held-out error on %d synthetic spectra: ΔE00 median=%.2f  p95=%.2f  max=%.2f | Δxy median=%.5f p95=%.5f\n",
		r.MapD65.TestSamples, q.MedianDE00, q.P95DE00, q.MaxDE00, q.MedianXY, q.P95XY)
	fmt.Fprintln(w, "(error measured against direct 10° integration of the same synthetic spectra)")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "A. Spectral samples: BOTH observers computed directly from spectra (D65)")
	fmt.Fprintln(w, repeat("-", 72))
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SAMPLE\tx2\ty2\tx10\ty10\tΔxy\tΔE00(2°vs10°)\tmap resid ΔE00")
	for _, row := range r.Spectral {
		fmt.Fprintf(tw, "%s\t%.4f\t%.4f\t%.4f\t%.4f\t%.5f\t%.2f\t%.2f\n",
			row.Name, row.X2, row.Y2, row.X10, row.Y10, row.DeltaXY, row.DeltaE00, row.MapResidualDE00)
	}
	tw.Flush()
	fmt.Fprintln(w)

	fmt.Fprintln(w, "B. sRGB swatches: only 2° XYZ exists; 10° coordinates are EMPIRICAL estimates")
	fmt.Fprintln(w, repeat("-", 72))
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SWATCH\thex\tx2\ty2\tx10(est)\ty10(est)\tΔxy(est)\tΔE00(est)\tSTATUS")
	for _, row := range r.Swatches {
		status := "approximate"
		if row.Uncertain {
			status = "UNCERTAIN"
		}
		fmt.Fprintf(tw, "%s\t%s\t%.4f\t%.4f\t%.4f\t%.4f\t%.5f\t%.2f\t%s\n",
			row.Name, row.Hex, row.X2, row.Y2, row.X10, row.Y10, row.DeltaXY, row.DeltaE00, status)
	}
	tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ΔE00 = CIEDE2000 between L*a*b* under each observer's own integrated D65 white.")
	fmt.Fprintln(w, "Cross-observer ΔE values are illustrative indicators, not a standardized metric.")
}

// WriteCertaintyText renders ICC profile provenance and uncertainty.
func WriteCertaintyText(w io.Writer, c Certainty) {
	fmt.Fprintln(w, "ICC profile / provenance")
	fmt.Fprintln(w, repeat("-", 72))
	if c.ProfilePath == "" {
		fmt.Fprintln(w, "profile: (none supplied)")
	} else {
		fmt.Fprintf(w, "profile: %s\n", c.ProfilePath)
	}
	if c.Profile != nil {
		p := c.Profile
		fmt.Fprintf(w, "  ICC version %s, class %s, colorSpace=%s PCS=%s, tags=%v\n",
			p.Version, p.DeviceClass, p.ColorSpace, p.PCS, p.TagSignatures)
		if p.MediaWhitePoint != nil {
			x, y, _ := p.MediaWhitePoint.Chromaticity()
			fmt.Fprintf(w, "  wtpt XYZ=(%.4f,%.4f,%.4f) -> xy=(%.4f,%.4f)\n",
				p.MediaWhitePoint.X, p.MediaWhitePoint.Y, p.MediaWhitePoint.Z, x, y)
		} else {
			fmt.Fprintln(w, "  wtpt: <missing>")
		}
	}
	fmt.Fprintf(w, "  observer recorded in profile: %v\n", c.ObserverRecorded)
	fmt.Fprintf(w, "  white point class: %s (supported=%v)\n", c.WhitePoint, c.WhiteSupported)
	status := "OK (empirical approximation still applies)"
	if c.Uncertain {
		status = "UNCERTAIN"
	}
	fmt.Fprintf(w, "  status: %s\n", status)
	for _, reason := range c.Reasons {
		fmt.Fprintf(w, "   - %s\n", reason)
	}
}

// WriteMapResultText renders a one-off mapping query.
func WriteMapResultText(w io.Writer, r *MapResult) {
	fmt.Fprintln(w, "Observer mapping query (experimental)")
	fmt.Fprintln(w, repeat("=", 72))
	fmt.Fprintln(w, "⚠ "+r.Disclaimer)
	fmt.Fprintln(w)
	WriteCertaintyText(w, r.Profile)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "input %s, direction %s, white %s\n", r.Input, r.Direction, r.WhitePoint)
	if r.Direction == "10to2" {
		fmt.Fprintf(w, "XYZ_10  = (%8.3f, %8.3f, %8.3f)  xy=(%.4f, %.4f)   [source]\n",
			r.XYZ10.X, r.XYZ10.Y, r.XYZ10.Z, r.X10, r.Y10)
		fmt.Fprintf(w, "XYZ_2e  = (%8.3f, %8.3f, %8.3f)  xy=(%.4f, %.4f)   [empirical estimate]\n",
			r.XYZ2.X, r.XYZ2.Y, r.XYZ2.Z, r.X2, r.Y2)
	} else {
		fmt.Fprintf(w, "XYZ_2   = (%8.3f, %8.3f, %8.3f)  xy=(%.4f, %.4f)\n",
			r.XYZ2.X, r.XYZ2.Y, r.XYZ2.Z, r.X2, r.Y2)
		fmt.Fprintf(w, "XYZ_10e = (%8.3f, %8.3f, %8.3f)  xy=(%.4f, %.4f)   [empirical estimate]\n",
			r.XYZ10.X, r.XYZ10.Y, r.XYZ10.Z, r.X10, r.Y10)
	}
	fmt.Fprintf(w, "Δxy=%.5f   ΔE00 predicted observer difference=%.2f\n", r.DeltaXY, r.DeltaE00)
	fmt.Fprintf(w, "map held-out quality: median ΔE00=%.2f p95=%.2f\n",
		r.MapQuality.MedianDE00, r.MapQuality.P95DE00)
	if r.Uncertain {
		fmt.Fprintln(w, "STATUS: UNCERTAIN")
		if r.Note != "" {
			fmt.Fprintln(w, "  "+r.Note)
		}
	} else {
		fmt.Fprintln(w, "STATUS: provenance OK; value remains an empirical approximation, not a standard conversion")
	}
}

func writeMatrix(w io.Writer, m [3][3]float64) {
	for i := 0; i < 3; i++ {
		fmt.Fprintf(w, "  [%9.5f %9.5f %9.5f]\n", m[i][0], m[i][1], m[i][2])
	}
}

func repeat(s string, n int) string {
	out := ""
	for len(out) < n {
		out += s
	}
	return out[:n]
}
