package identity

import (
	"testing"

	"mkvidentity/timepoint"
)

func TestMigrateUsesUIDsInsteadOfLineOrder(t *testing.T) {
	source := &Inventory{
		Tracks: []Track{
			{UID: 101, Number: 1, Type: TrackAudio, Language: "eng", DefaultSet: true, Default: true},
			{UID: 102, Number: 2, Type: TrackAudio, Language: "fra", DefaultSet: true, Default: false},
		},
		Chapters: []Chapter{{
			UID: 700, EditionUID: 500, Start: timepoint.Resolved(10),
			TrackUIDs: []uint64{101},
		}},
	}
	target := &Inventory{
		Tracks: []Track{
			{UID: 202, Number: 1, Type: TrackAudio, Language: "fra", DefaultSet: true, Default: false},
			{UID: 201, Number: 2, Type: TrackAudio, Language: "eng", DefaultSet: true, Default: true},
		},
	}
	report := Migrate(source, target, Options{
		TrackUIDMap:         map[uint64]uint64{101: 201, 102: 202},
		CopyMissingChapters: true,
		MigrateLanguage:     true,
		MigrateDefaultFlags: true,
	})

	if len(report.UnresolvedRefs) != 0 {
		t.Fatalf("unexpected unresolved refs: %+v", report.UnresolvedRefs)
	}
	if len(report.Patch.TrackPatches) != 0 {
		t.Fatalf("identical metadata should need no patch: %+v", report.Patch.TrackPatches)
	}
	if len(report.Patch.Chapters) != 1 || report.Patch.Chapters[0].TrackUIDs[0] != 201 {
		t.Fatalf("chapter ref not remapped: %+v", report.Patch.Chapters)
	}
	if report.Patch.Chapters[0].UID != 700 {
		t.Fatal("copied chapter should preserve stable ChapterUID")
	}
}

func TestDefaultIdentityMovesWhenTargetLineChanges(t *testing.T) {
	source := &Inventory{Tracks: []Track{
		{UID: 1, Number: 1, Type: TrackAudio, DefaultSet: true, Default: false},
		{UID: 2, Number: 2, Type: TrackAudio, Language: "chi", DefaultSet: true, Default: true},
	}}
	target := &Inventory{Tracks: []Track{
		{UID: 9, Number: 1, Type: TrackAudio, DefaultSet: true, Default: true},
		{UID: 2, Number: 2, Type: TrackAudio, Language: "jpn", DefaultSet: true, Default: false},
	}}
	report := Migrate(source, target, Options{
		MigrateLanguage:       true,
		MigrateDefaultFlags:   true,
		ClearCompetingDefault: true,
	})

	var defaultPatch *TrackPatch
	for i := range report.Patch.TrackPatches {
		if report.Patch.TrackPatches[i].TargetUID == 2 {
			defaultPatch = &report.Patch.TrackPatches[i]
		}
	}
	if defaultPatch == nil || !defaultPatch.SetDefault || !defaultPatch.Default {
		t.Fatalf("migrated default patch missing: %+v", report.Patch.TrackPatches)
	}
	var clearPatch *TrackPatch
	for i := range report.Patch.TrackPatches {
		if report.Patch.TrackPatches[i].TargetUID == 9 {
			clearPatch = &report.Patch.TrackPatches[i]
		}
	}
	if clearPatch == nil || clearPatch.Default {
		t.Fatalf("competing target default was not cleared: %+v", report.Patch.TrackPatches)
	}
}

func TestMissingTrackAndDeletedTimeAreMarked(t *testing.T) {
	source := &Inventory{Chapters: []Chapter{{
		UID: 7, Start: timepoint.Unresolved("99:99", timepoint.StatusUnparseable, "bad"),
		TrackUIDs: []uint64{404},
	}}}
	target := &Inventory{}
	timeline := timepoint.TimelineMap{Cuts: []timepoint.Cut{{StartNS: 5, EndNS: 15}}}
	source.Chapters = append(source.Chapters, Chapter{
		UID: 8, Start: timepoint.Resolved(10), TrackUIDs: []uint64{404},
	})
	report := Migrate(source, target, Options{Timeline: &timeline})
	if len(report.UnresolvedTimes) == 0 {
		t.Fatal("expected malformed and deleted time issues")
	}
	if len(report.UnresolvedRefs) == 0 {
		t.Fatal("expected unresolved track reference")
	}
	foundDeleted := false
	for _, issue := range report.UnresolvedTimes {
		if issue.Code == "chapter_start_unresolved" && issue.Message != "" {
			for _, migration := range report.Chapters {
				if migration.OldUID == 8 && migration.Start.Status == timepoint.StatusRemoved {
					foundDeleted = true
				}
			}
		}
	}
	if !foundDeleted {
		t.Fatalf("deleted time not marked: %+v", report.Chapters)
	}
}
