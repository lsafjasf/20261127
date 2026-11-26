package hdrframe

func summarize(report *Report) Summary {
	summary := Summary{
		TargetFrames:  len(report.Frames),
		DuplicateKeys: len(report.Duplicates),
		SequenceGaps:  len(report.SequenceGaps),
	}
	for _, duplicate := range report.Duplicates {
		if !duplicate.Identical {
			summary.DuplicateConflicts++
		}
	}
	for _, frame := range report.Frames {
		switch frame.Status {
		case "matched":
			summary.Matched++
		case "mismatch":
			summary.Mismatched++
		default:
			summary.Unverified++
		}
		if frame.Applied == nil || !frame.Applied.Verified {
			if frame.Status != "unverified" {
				summary.Unverified++
			}
		}
		for _, issue := range frame.Issues {
			switch issue.Code {
			case "missing_exact_frame_metadata":
				summary.MissingExactRecord++
			case "scene_id_mismatch", "expected_scene_id_mismatch":
				summary.SceneMismatches++
			}
		}
	}
	return summary
}
