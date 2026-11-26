package align

import (
	"fmt"
	"io"
)

// WriteText writes a compact human-readable alignment summary.
func WriteText(w io.Writer, report *Report) error {
	if _, err := fmt.Fprintf(w, "MASTER: %s (%s)\n", report.MasterDevice, report.Timeline); err != nil {
		return err
	}
	for _, device := range report.Devices {
		if _, err := fmt.Fprintf(w, "\nDEVICE %s%s\n", device.ID, masterLabel(device)); err != nil {
			return err
		}
		for _, segment := range device.Segments {
			if _, err := fmt.Fprintf(w, "  SEGMENT %s [%.3f, %.3f] status=%s\n",
				segment.ID, segment.LocalStart, segment.LocalEnd, segment.Status); err != nil {
				return err
			}
			if segment.Status == "master" {
				fmt.Fprintf(w, "    identity mapping; master reference\n")
			} else {
				printFirstTrusted(w, segment)
				printDrift(w, segment)
			}
			if err := printMap(w, segment); err != nil {
				return err
			}
			if err := printUnknown(w, segment); err != nil {
				return err
			}
		}
	}
	if len(report.Diagnostics) > 0 {
		if _, err := fmt.Fprintf(w, "\nDIAGNOSTICS\n"); err != nil {
			return err
		}
		for _, item := range report.Diagnostics {
			if _, err := fmt.Fprintf(w, "  %s %s: %s\n", item.Severity, item.Code, item.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func masterLabel(device DeviceInfo) string {
	if device.Master {
		return " [master]"
	}
	return ""
}

func printFirstTrusted(w io.Writer, segment SegmentReport) {
	if segment.FirstTrusted != nil {
		first := segment.FirstTrusted
		fmt.Fprintf(w, "    first trusted: %s (%s) local=%.3f master=%.3f offset=%+.3f s conf=%.2f\n",
			first.Event, first.Kind, first.Local, first.Master, first.OffsetSec, first.Confidence)
		return
	}
	if segment.OffsetAtStart != nil {
		fmt.Fprintf(w, "    first trusted: none (provisional offset at start %+.3f s)\n",
			*segment.OffsetAtStart)
	}
}

func printDrift(w io.Writer, segment SegmentReport) {
	if segment.DriftPPM == nil {
		fmt.Fprintf(w, "    drift: unobservable\n")
		return
	}
	fmt.Fprintf(w, "    drift: %+.1f ppm (confirmed=%t", *segment.DriftPPM, segment.DriftConfirmed)
	if segment.Fit != nil {
		fmt.Fprintf(w, ", anchors=%d, rms=%.1f ms", segment.Fit.AnchorCount, segment.Fit.RMSResidualMS)
	}
	fmt.Fprintf(w, ")\n")
}

func printMap(w io.Writer, segment SegmentReport) error {
	if len(segment.Map) == 0 {
		_, err := fmt.Fprintf(w, "    map: none\n")
		return err
	}
	_, err := fmt.Fprintf(w, "    map:\n")
	if err != nil {
		return err
	}
	for _, piece := range segment.Map {
		if _, err := fmt.Fprintf(w, "      local [%.3f, %.3f) -> master [%.3f, %.3f) err<=%.1f ms\n",
			piece.LocalStart, piece.LocalEnd, piece.MasterStart, piece.MasterEnd, piece.MaxErrorMS); err != nil {
			return err
		}
	}
	return nil
}

func printUnknown(w io.Writer, segment SegmentReport) error {
	if len(segment.Indeterminate) == 0 {
		_, err := fmt.Fprintf(w, "    undetermined: none\n")
		return err
	}
	_, err := fmt.Fprintf(w, "    undetermined:\n")
	if err != nil {
		return err
	}
	for _, interval := range segment.Indeterminate {
		if _, err := fmt.Fprintf(w, "      local [%.3f, %.3f): %s\n",
			interval.LocalStart, interval.LocalEnd, interval.Reason); err != nil {
			return err
		}
	}
	return nil
}
