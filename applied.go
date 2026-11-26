package hdrframe

func resolveApplied(
	event PlayerEvent,
	target Target,
	targetSeconds, tolerance, fps float64,
	policy Policy,
	selected *resolvedRecord,
	lastApplied *resolvedRecord,
	records []resolvedRecord,
	gaps []SequenceGap,
) *AppliedRef {
	if playerEventPresent(event) {
		version := event.AppliedVersion
		if version == "" {
			version = event.Version
		}
		checksum := event.AppliedChecksum
		if checksum == "" {
			checksum = event.Checksum
		}
		var sequence int64
		if event.AppliedSequence != nil {
			sequence = *event.AppliedSequence
		} else if event.Sequence != nil {
			sequence = *event.Sequence
		}

		probe := &AppliedRef{}
		probe.Version = version
		probe.Checksum = checksum
		probe.Sequence = sequence
		if record := findRecordByApplied(probe, records); record != nil {
			ref := versionRef(record, target.Frame, targetSeconds, tolerance, fps)
			return &AppliedRef{
				VersionRef:     ref,
				EvidenceSource: evidenceSource(event.Source, "player_log"),
				Evidence:       firstNonEmpty(event.Evidence, event.Note),
				Verified:       true,
				Reason:         "player/event log names this version",
			}
		}

		effective := event.PTSSeconds
		return &AppliedRef{
			VersionRef: VersionRef{
				Version: version, Sequence: sequence, Checksum: checksum,
				EffectiveSeconds:  effective,
				EffectiveTimecode: formatTimecode(effective, fps),
				SelectionBasis:    "player_evidence_unresolved_metadata",
			},
			EvidenceSource: evidenceSource(event.Source, "player_log"),
			Evidence:       firstNonEmpty(event.Evidence, event.Note),
			Verified:       true,
			Reason:         "player reports a version/checksum absent from supplied metadata",
		}
	}

	if policy.MissingMetadata == "default" && !hasExactFrameRecord(target.Frame, records) {
		ref := defaultAppliedRef()
		return &ref
	}
	if policy.MissingMetadata == "none" && !hasExactFrameRecord(target.Frame, records) {
		return nil
	}

	candidate := selected
	reason := "latest effective metadata applied"
	exact := hasExactFrameRecord(target.Frame, records)
	if !exact && policy.MissingMetadata == "hold_last" && lastApplied != nil {
		candidate = lastApplied
		reason = "no frame record; player is inferred to hold its previous applied version"
	}
	if selected != nil && policy.SequenceGap == "reject" && gapStartsWithSelected(selected, gaps) && lastApplied != nil {
		candidate = lastApplied
		reason = "sequence jump rejected by policy; previous version remains active"
	}
	if candidate == nil {
		return nil
	}

	ref := versionRef(candidate, target.Frame, targetSeconds, tolerance, fps)
	return &AppliedRef{
		VersionRef:     ref,
		EvidenceSource: "inferred_" + policy.MissingMetadata,
		Verified:       false,
		Reason:         reason,
	}
}

func playerEventPresent(event PlayerEvent) bool {
	return event.AppliedVersion != "" || event.Version != "" ||
		event.AppliedSequence != nil || event.Sequence != nil ||
		event.AppliedChecksum != "" || event.Checksum != "" ||
		event.Source != "" || event.Evidence != "" || event.Note != ""
}

func evidenceSource(source, fallback string) string {
	if source != "" {
		return source
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultAppliedRef() AppliedRef {
	return AppliedRef{
		VersionRef: VersionRef{
			Version:  "player-default-hdr10-fallback",
			Checksum: "", SelectionBasis: "built_in_default",
		},
		EvidenceSource: "policy_default",
		Verified:       false,
		Reason:         "missing dynamic metadata selects the player/system default",
	}
}

func gapStartsWithSelected(selected *resolvedRecord, gaps []SequenceGap) bool {
	if selected == nil {
		return false
	}
	sequence := selected.meta.sequenceValue()
	for _, gap := range gaps {
		if sequence == gap.BeforeSequence {
			return true
		}
	}
	return false
}
