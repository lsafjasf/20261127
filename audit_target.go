package hdrframe

import (
	"fmt"
	"math"
)

func auditTarget(
	target Target,
	fps float64,
	policy Policy,
	records []resolvedRecord,
	expectedRecords []resolvedRecord,
	duplicates []Duplicate,
	gaps []SequenceGap,
	event PlayerEvent,
	lastApplied *resolvedRecord,
) FrameReport {
	seconds, timecode, err := targetTime(target, fps)
	if err != nil {
		seconds = float64(target.Frame) / fps
		timecode = formatTimecode(seconds, fps)
	}

	report := FrameReport{
		Frame: target.Frame, PTSSeconds: seconds, Timecode: timecode,
		Label: target.Label, ExpectedSceneID: target.ExpectedSceneID,
		Status: "unverified", TimelineVerdict: "no_decision",
	}
	if target.ExpectedSceneID == nil {
		report.ExpectedSceneID = target.ExpectedScene
	}
	if err != nil {
		report.Issues = append(report.Issues, Issue{
			Code: "invalid_target_timecode", Severity: "error", Frame: target.Frame, Message: err.Error(),
		})
	}

	tolerance := policy.TimeToleranceSecs

	expected, expectedAmbiguous := selectResolved(expectedRecords, seconds, tolerance, policy)
	if expected != nil {
		ref := versionRef(expected, target.Frame, seconds, tolerance, fps)
		report.Expected = &ref
		if expectedAmbiguous {
			report.Issues = append(report.Issues, Issue{
				Code: "ambiguous_expected_duplicate", Severity: "error", Frame: target.Frame,
				Message: "source/expected metadata has conflicting duplicates",
			})
		}
	}

	selected, ambiguous := selectResolved(records, seconds, tolerance, policy)
	if selected != nil {
		if ambiguous {
			report.Issues = append(report.Issues, Issue{
				Code: "ambiguous_duplicate", Severity: "error", Frame: target.Frame,
				Message: "conflicting duplicate metadata records and duplicate policy is error",
			})
		}
		ref := versionRef(selected, target.Frame, seconds, tolerance, fps)
		report.Selected = &ref
		report.TimelineVerdict = "selected_by_" + ref.SelectionBasis
	}

	exact := hasExactFrameRecord(target.Frame, records)
	if !exact {
		report.Issues = append(report.Issues, Issue{
			Code: "missing_exact_frame_metadata", Severity: "warning", Frame: target.Frame,
			Message: fmt.Sprintf("no metadata record is explicitly tagged for frame %d; active version must be carried or held", target.Frame),
		})
	}

	if report.Expected != nil {
		if issue := sceneMismatchIssue(target.Frame, "expected_scene_id_mismatch", report.ExpectedSceneID, report.Expected.SceneID, "source metadata"); issue != nil {
			report.Issues = append(report.Issues, *issue)
		}
	}
	if report.Selected != nil {
		if issue := sceneMismatchIssue(target.Frame, "scene_id_mismatch", report.ExpectedSceneID, report.Selected.SceneID, "transcoded metadata"); issue != nil {
			report.Issues = append(report.Issues, *issue)
		}
	}

	for _, dup := range duplicates {
		if report.Selected != nil && dup.Key == recordKey(*selected) && !dup.Identical {
			severity := "warning"
			code := "duplicate_conflict_resolved"
			if policy.Duplicate == "error" {
				severity = "error"
				code = "ambiguous_duplicate"
			}
			report.Issues = append(report.Issues, Issue{
				Code: code, Severity: severity, Frame: target.Frame,
				Message:  fmt.Sprintf("duplicate versions %v; applied selection policy %q", dup.Versions, dup.Resolution),
				Expected: dup.ChosenVersion, Actual: stringsJoin(dup.Versions, ","),
			})
		}
	}

	if selected != nil {
		seq := selected.meta.sequenceValue()
		for _, gap := range gaps {
			if seq == gap.BeforeSequence {
				report.Issues = append(report.Issues, Issue{
					Code: "sequence_jump", Severity: "warning", Frame: target.Frame,
					Message:  fmt.Sprintf("sequence jumps %d -> %d; missing sequence numbers %v", gap.AfterSequence, gap.BeforeSequence, gap.Missing),
					Expected: fmt.Sprintf("continuous after %d", gap.AfterSequence),
					Actual:   fmt.Sprintf("%d", gap.BeforeSequence),
				})
			}
		}
	}

	applied := resolveApplied(event, target, seconds, tolerance, fps, policy, selected, lastApplied, records, gaps)
	report.Applied = applied
	if applied != nil {
		applied.MatchesSelected = versionsMatch(&applied.VersionRef, report.Selected)
		if report.Expected != nil {
			applied.MatchesExpected = versionsMatch(&applied.VersionRef, report.Expected)
			if !versionsMatch(report.Selected, report.Expected) {
				report.Issues = append(report.Issues, Issue{
					Code: "metadata_delivery_drift", Severity: "error", Frame: target.Frame,
					Message: fmt.Sprintf("source metadata expects %s but transcoded timeline can select %s",
						selectedVersion(report.Expected), selectedVersion(report.Selected)),
					Expected: selectedVersion(report.Expected), Actual: selectedVersion(report.Selected),
				})
			}
		}
		paramBaseline := report.Expected
		if paramBaseline == nil {
			paramBaseline = report.Selected
		}
		report.ParameterDiffs = diffVersionParams(paramBaseline, &applied.VersionRef)
		correct := applied.MatchesSelected
		correctVersion := report.Selected
		if report.Expected != nil {
			correct = applied.MatchesExpected
			correctVersion = report.Expected
		}
		if correct {
			report.Status = "matched"
		} else {
			report.Status = "mismatch"
			code := "applied_version_mismatch"
			if applied.MatchesSelected && report.Expected != nil {
				code = "stale_or_wrong_metadata_applied"
			}
			report.Issues = append(report.Issues, Issue{
				Code: code, Severity: "error", Frame: target.Frame,
				Message: fmt.Sprintf("player applied %s while required version is %s",
					applied.Version, selectedVersion(correctVersion)),
				Expected: selectedVersion(correctVersion), Actual: applied.Version,
			})
		}
		if !applied.Verified && report.Status == "matched" {
			report.TimelineVerdict = "inferred_application_matches_required_version"
		} else if !applied.Verified {
			report.TimelineVerdict = "inferred_application_differs"
		} else if report.Status == "matched" {
			report.TimelineVerdict = "player_confirms_required_version"
		} else if applied.MatchesSelected && report.Expected != nil {
			report.TimelineVerdict = "player_confirms_transcoded_stale_version"
		} else {
			report.TimelineVerdict = "player_evidence_confirms_wrong_version"
		}
	} else if policy.MissingMetadata == "default" {
		ref := defaultAppliedRef()
		report.Applied = &ref
		report.Status = "mismatch"
		report.TimelineVerdict = "player_default_parameters_expected"
	}

	return report
}

func versionRef(record *resolvedRecord, targetFrame int64, targetTime, tolerance, fps float64) VersionRef {
	effectiveTC := formatTimecode(record.effective, fps)
	basis := "effective_time"
	frame, hasFrame := record.meta.frameValue()
	if hasFrame && frame == targetFrame && almostEqual(record.effective, targetTime, tolerance) {
		basis = "exact_frame_and_effective_time"
	} else if hasFrame && frame == targetFrame {
		basis = "exact_frame"
	} else if record.effective < targetTime-tolerance {
		basis = "prior_effective_hold_candidate"
	}
	return VersionRef{
		RecordID: record.meta.ID, Version: record.version,
		Sequence: record.meta.sequenceValue(), Frame: record.meta.Frame,
		SceneID: record.meta.sceneValue(), SceneFrameIndex: record.meta.SceneFrameIndex,
		EffectiveSeconds: record.effective, EffectiveTimecode: effectiveTC,
		Checksum: record.fingerprint, SelectionBasis: basis,
		Source: record.meta.Source, Highlight: record.meta.Highlight, Color: record.meta.Color,
	}
}

func almostEqual(a, b, tolerance float64) bool {
	if tolerance <= 0 {
		tolerance = 1e-9
	}
	return math.Abs(a-b) <= tolerance
}

func hasExactFrameRecord(frame int64, records []resolvedRecord) bool {
	for _, record := range records {
		if f, ok := record.meta.frameValue(); ok && f == frame {
			return true
		}
		if record.meta.sequenceValue() == frame && record.meta.Frame == nil && record.meta.FrameIndex == nil {
			return true
		}
	}
	return false
}

func selectedVersion(ref *VersionRef) string {
	if ref == nil {
		return "<none>"
	}
	return ref.Version
}

func versionsMatch(a, b *VersionRef) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Version != "" && b.Version != "" && a.Version == b.Version {
		return true
	}
	if a.Checksum != "" && b.Checksum != "" && a.Checksum == b.Checksum {
		return true
	}
	return false
}

func ptrEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sceneMismatchIssue(frame int64, code string, expected, actual *int64, where string) *Issue {
	if expected == nil || actual == nil || *expected == *actual {
		return nil
	}
	return &Issue{
		Code: code, Severity: "error", Frame: frame, SceneID: actual,
		Message: fmt.Sprintf("frame belongs to scene %s but %s carries scene %s",
			int64PtrString(expected), where, int64PtrString(actual)),
		Expected: int64PtrString(expected), Actual: int64PtrString(actual),
	}
}

func stringsJoin(values []string, sep string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += sep
		}
		out += value
	}
	return out
}
