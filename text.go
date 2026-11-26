package tearcheck

import (
	"fmt"
	"strings"
)

// FormatTextReport renders a compact human-review summary.
func FormatTextReport(report *Report) string {
	if report == nil {
		return ""
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "Tear analysis: %s\n", report.Source)
	if report.Width > 0 && report.Height > 0 {
		fmt.Fprintf(&builder, "Geometry: %dx%d codec=%s\n", report.Width, report.Height, report.Codec)
	}
	fmt.Fprintf(&builder, "Overall: %s; frames: %d; findings: %d; manual review: %d\n",
		report.Summary.Overall, report.Summary.FramesAnalyzed, report.Summary.Findings, report.Summary.ManualReview)

	for _, finding := range report.Findings {
		location := fmt.Sprintf("frame=%d", finding.FrameIndex)
		if finding.Time > 0 {
			location += fmt.Sprintf(" t=%.3fs", finding.Time)
		}
		if finding.SeamY > 0 {
			location += fmt.Sprintf(" y=%d", finding.SeamY)
		}
		fmt.Fprintf(&builder, "- [%s/%s] %s score=%.2f margin=%.2f\n",
			finding.Cause, finding.Confidence, location, finding.Score, finding.Margin)
		if finding.Manual != nil {
			causes := make([]string, 0, len(finding.Manual.PossibleCauses))
			for _, cause := range finding.Manual.PossibleCauses {
				causes = append(causes, string(cause))
			}
			fmt.Fprintf(&builder, "  manual: %s\n", finding.Manual.Reason)
			fmt.Fprintf(&builder, "  possible: %s\n", strings.Join(causes, ", "))
		}
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(&builder, "warning: %s\n", warning)
	}
	return builder.String()
}
