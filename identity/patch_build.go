package identity

import "fmt"

func buildTrackPatches(source, target *Inventory, opts Options, matches trackMatchResult, report *Report) {
	for i, oldTrack := range source.Tracks {
		targetIndex, ok := matches.targetIndex[i]
		if !ok {
			continue
		}
		newTrack := target.Tracks[targetIndex]
		migration := &report.Tracks[i]
		patch := TrackPatch{TargetUID: newTrack.UID}
		patchNeeded := false

		if opts.MigrateLanguage {
			if oldTrack.Language != "" && oldTrack.Language != newTrack.Language {
				migration.Language = StringChange{From: newTrack.Language, To: oldTrack.Language, Changed: true}
				patch.Language, patch.SetLanguage = oldTrack.Language, true
				patchNeeded = true
			}
			if oldTrack.LanguageIETF != "" && oldTrack.LanguageIETF != newTrack.LanguageIETF {
				migration.LanguageIETF = StringChange{
					From: newTrack.LanguageIETF, To: oldTrack.LanguageIETF, Changed: true,
				}
				patch.LanguageIETF, patch.SetLanguageIETF = oldTrack.LanguageIETF, true
				patchNeeded = true
			}
		}

		if opts.MigrateDefaultFlags {
			oldDefault := effectiveDefault(oldTrack)
			newDefault := effectiveDefault(newTrack)
			migration.Default = BoolChange{From: newDefault, To: oldDefault, Changed: oldDefault != newDefault}
			if oldDefault != newDefault {
				patch.Default, patch.SetDefault = oldDefault, true
				patchNeeded = true
			}
		}

		if patchNeeded {
			report.Patch.TrackPatches = append(report.Patch.TrackPatches, patch)
		}
	}
}

func effectiveDefault(track Track) bool {
	if track.DefaultSet {
		return track.Default
	}
	return true
}

func clearCompetingDefaults(target *Inventory, matches trackMatchResult, report *Report) {
	matchedTarget := map[uint64]bool{}
	defaultTypes := map[TrackType]bool{}
	for _, migration := range report.Tracks {
		if migration.NewUID != 0 && migration.Default.To {
			matchedTarget[migration.NewUID] = true
			for _, track := range target.Tracks {
				if track.UID == migration.NewUID {
					defaultTypes[track.Type] = true
				}
			}
		}
	}
	for _, track := range target.Tracks {
		if !defaultTypes[track.Type] || matchedTarget[track.UID] || !effectiveDefault(track) {
			continue
		}
		alreadyPatched := false
		for i := range report.Patch.TrackPatches {
			if report.Patch.TrackPatches[i].TargetUID == track.UID {
				report.Patch.TrackPatches[i].Default = false
				report.Patch.TrackPatches[i].SetDefault = true
				alreadyPatched = true
			}
		}
		if !alreadyPatched {
			report.Patch.TrackPatches = append(report.Patch.TrackPatches, TrackPatch{
				TargetUID: track.UID, Default: false, SetDefault: true,
			})
		}
		report.Issues = append(report.Issues, Issue{
			Code: "competing_default_cleared", Severity: SeverityWarning,
			Subject: track.StableRef(),
			Message: fmt.Sprintf("cleared target default flag on track type %d so migrated default identity wins", track.Type),
		})
	}
}

func buildChapterMigrations(source *Inventory, opts Options, tracks trackMatchResult, chapters chapterMatchResult, report *Report) {
	for i, oldChapter := range source.Chapters {
		migration := &report.Chapters[i]
		migrated := oldChapter
		migrated.TrackUIDs = nil

		if opts.Timeline != nil {
			migration.Start = opts.Timeline.MapPoint(oldChapter.Start)
			if oldChapter.End != nil {
				end := opts.Timeline.MapPoint(*oldChapter.End)
				migration.End = &end
			}
		}
		migrated.Start = migration.Start
		migrated.End = migration.End
		if !migration.Start.Ok() && opts.KeepRawUnmappedTimes && oldChapter.Start.Ok() {
			migrated.Start = oldChapter.Start
			report.Issues = append(report.Issues, Issue{
				Code: "unresolved_time_raw_kept", Severity: SeverityWarning,
				Subject: migration.OldRef, Field: "ChapterTimeStart",
				Message: "target time is unresolved; output keeps original EBML value only because raw-keep was requested",
			})
		}
		if migration.End != nil && !migration.End.Ok() {
			if opts.KeepRawUnmappedTimes && oldChapter.End != nil && oldChapter.End.Ok() {
				migrated.End = oldChapter.End
			} else {
				migrated.End = migration.End
			}
		}

		for _, oldTrackUID := range oldChapter.TrackUIDs {
			refMigration := RefMigration{
				Type: "chapter-track", FromUID: oldTrackUID, FromRef: TrackRef(oldTrackUID),
			}
			newTrackUID, ok := tracks.uid[oldTrackUID]
			if !ok {
				refMigration.Basis = BasisMissing
				refMigration.Status = "unresolved"
				report.Issues = append(report.Issues, Issue{
					Code: "unresolved_track_reference", Severity: SeverityUnresolved,
					Subject: ChapterRef(oldChapter.UID), Field: "ChapterTrackUID",
					Message: fmt.Sprintf("source TrackUID %d has no target identity", oldTrackUID),
				})
				migrated.TrackUIDs = append(migrated.TrackUIDs, oldTrackUID)
			} else {
				refMigration.ToUID, refMigration.ToRef = newTrackUID, TrackRef(newTrackUID)
				refMigration.Basis, refMigration.Status = BasisUID, "rewritten"
				if basis := basisForTrackUID(report, oldTrackUID); basis != "" {
					refMigration.Basis = basis
				}
				migrated.TrackUIDs = append(migrated.TrackUIDs, newTrackUID)
			}
			migration.Refs = append(migration.Refs, refMigration)
		}

		if oldChapter.ParentUID != 0 {
			parentRef := RefMigration{
				Type: "chapter-parent", FromUID: oldChapter.ParentUID,
				FromRef: ChapterRef(oldChapter.ParentUID),
			}
			newParent, ok := chapters.uid[oldChapter.ParentUID]
			if !ok {
				parentRef.Status, parentRef.Basis = "unresolved", BasisMissing
				report.Issues = append(report.Issues, Issue{
					Code: "unresolved_parent_reference", Severity: SeverityUnresolved,
					Subject: ChapterRef(oldChapter.UID), Field: "ChapterAtomUID",
					Message: fmt.Sprintf("parent ChapterUID %d has no target identity", oldChapter.ParentUID),
				})
			} else {
				parentRef.ToUID, parentRef.ToRef = newParent, ChapterRef(newParent)
				parentRef.Status, parentRef.Basis = "rewritten", migration.Basis
				migrated.ParentUID = newParent
			}
			migration.Refs = append(migration.Refs, parentRef)
		}

		if oldChapter.EditionUID != 0 {
			editionRef := RefMigration{
				Type: "chapter-edition", FromUID: oldChapter.EditionUID,
				FromRef: EditionRef(oldChapter.EditionUID),
			}
			newEdition := oldChapter.EditionUID
			if mapped, ok := opts.EditionUIDMap[oldChapter.EditionUID]; ok {
				newEdition = mapped
				editionRef.Basis = BasisExplicit
			} else {
				editionRef.Basis = BasisUID
			}
			editionRef.ToUID, editionRef.ToRef = newEdition, EditionRef(newEdition)
			editionRef.Status = "rewritten"
			migrated.EditionUID = newEdition
			migration.Refs = append(migration.Refs, editionRef)
		}

		if migration.NewUID != 0 {
			migrated.UID = migration.NewUID
		}
		migration.Migrated = migrated
		if migration.Basis == BasisCopied {
			report.Patch.Chapters = append(report.Patch.Chapters, migrated)
			report.Patch.ReplaceChapters = true
		}
	}
}

func basisForTrackUID(report *Report, oldUID uint64) MatchBasis {
	for _, track := range report.Tracks {
		if track.OldUID == oldUID {
			return track.Basis
		}
	}
	return ""
}

func ClassifyReport(report *Report) {
	report.UnresolvedTimes = nil
	report.UnresolvedRefs = nil
	for _, issue := range report.Issues {
		switch issue.Code {
		case "chapter_start_unresolved", "chapter_end_unresolved":
			report.UnresolvedTimes = append(report.UnresolvedTimes, issue)
		case "unresolved_track_reference", "unresolved_parent_reference", "dangling_chapter_track_ref":
			report.UnresolvedRefs = append(report.UnresolvedRefs, issue)
		}
	}
	for _, chapter := range report.Chapters {
		if !chapter.Start.Ok() {
			report.UnresolvedTimes = append(report.UnresolvedTimes, Issue{
				Code: "chapter_start_unresolved", Severity: SeverityUnresolved,
				Subject: chapter.OldRef, Message: chapter.Start.Reason,
			})
		}
		if chapter.End != nil && !chapter.End.Ok() {
			report.UnresolvedTimes = append(report.UnresolvedTimes, Issue{
				Code: "chapter_end_unresolved", Severity: SeverityUnresolved,
				Subject: chapter.OldRef, Message: chapter.End.Reason,
			})
		}
	}
}
