package iccproof

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
)

// WriteJSON writes an indented stable JSON report.
func WriteJSON(report *Report, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// WriteCSV writes one row for every sample × intent result.
func WriteCSV(report *Report, w io.Writer) error {
	cw := csv.NewWriter(w)
	header := []string{
		"intent", "sample_index", "sample_id", "x", "y",
		"classification", "boundary_stage", "source_representable",
		"media_in_gamut", "media_gamut_alarm", "device_clipped",
		"flags", "delta_e", "delta_e00", "delta_e76", "delta_e_reliable",
		"input_rgb", "media_color_space", "media_values", "media_codes", "error",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range report.Results {
		mediaIn := ""
		if r.Boundary.MediaInGamut != nil {
			mediaIn = strconv.FormatBool(*r.Boundary.MediaInGamut)
		}
		row := []string{
			string(r.Intent),
			strconv.Itoa(r.SampleIndex),
			r.SampleID,
			coordPtrString(r.X), coordPtrString(r.Y),
			string(r.Boundary.Classification),
			string(r.Boundary.Stage),
			strconv.FormatBool(r.Boundary.SourceRepresentable),
			mediaIn,
			strconv.FormatBool(r.Boundary.MediaGamutAlarm),
			strconv.FormatBool(r.Boundary.DeviceClipped),
			strings.Join(r.Boundary.Flags, ";"),
			deltaString(r.DeltaE),
			deltaString(r.DeltaE00),
			deltaString(r.DeltaE76),
			boolPtrString(r.DeltaEReliable),
			floatList(r.InputRGB),
			r.MediaColorSpace,
			floatList(r.MediaValues),
			intList(r.MediaCodes),
			r.Error,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteText writes a concise human-readable summary and worst samples.
func WriteText(report *Report, w io.Writer) error {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "ICC soft-proof report %s\n", report.SchemaVersion)
	fmt.Fprintf(tw, "source: %s (%s, sha256:%s)\n", profileLabel(report.SourceProfile), report.SourceProfile.ColorSpace, shortSHA(report.SourceProfile.SHA256))
	fmt.Fprintf(tw, "media:  %s (%s, sha256:%s)\n", profileLabel(report.MediaProfile), report.MediaProfile.ColorSpace, shortSHA(report.MediaProfile.SHA256))
	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "intent\tsamples\tsource_bad\tmedia_oog\tclipped\tDE_min\tDE_mean\tDE_max")
	for _, s := range report.Summaries {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%.3f\t%.3f\t%.3f\n",
			s.Intent, s.SampleCount, s.SourceUnrepresentable,
			s.MediaOutOfGamut, s.DeviceClipped,
			s.DeltaE.Min, s.DeltaE.Mean, s.DeltaE.Max)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(report.IntentComparisons) > 0 {
		fmt.Fprintln(w, "\nintent output differences (CIEDE2000 between proof results):")
		for _, c := range report.IntentComparisons {
			fmt.Fprintf(w, "  %s vs %s: samples=%d mean=%.3f max=%.3f\n",
				c.IntentA, c.IntentB, c.DeltaE.Count, c.DeltaE.Mean, c.DeltaE.Max)
		}
	}

	fmt.Fprintln(w, "\nper-sample:")
	for _, r := range report.Results {
		de := "n/a"
		if r.DeltaE00 != nil {
			de = fmt.Sprintf("%.3f", *r.DeltaE00)
		}
		fmt.Fprintf(w, "  [%s #%d %s] class=%s ΔE00=%s flags=%s\n",
			r.Intent, r.SampleIndex, r.SampleID, r.Boundary.Classification, de, strings.Join(r.Boundary.Flags, ","))
	}
	return nil
}

func profileLabel(p ProfileInfo) string {
	if p.Path != "" {
		return p.Path
	}
	if p.Backend != "" {
		return p.Backend
	}
	return "explicit-profile"
}

func shortSHA(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

func coordPtrString(v *int) string {
	if v == nil || *v < 0 {
		return ""
	}
	return strconv.Itoa(*v)
}

func deltaString(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 6, 64)
}

func boolPtrString(v *bool) string {
	if v == nil {
		return ""
	}
	return strconv.FormatBool(*v)
}

func floatList(values []float64) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.FormatFloat(v, 'f', 5, 64)
	}
	return strings.Join(parts, ";")
}

func intList(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ";")
}
