package identity

import (
	"fmt"

	"mkvidentity/timepoint"
)

type MatchBasis string

const (
	BasisUID         MatchBasis = "uid"
	BasisExplicit    MatchBasis = "explicit_map"
	BasisStringUID   MatchBasis = "string_uid"
	BasisFingerprint MatchBasis = "fingerprint_unique"
	BasisCopied      MatchBasis = "copied_preserve_uid"
	BasisAmbiguous   MatchBasis = "ambiguous"
	BasisMissing     MatchBasis = "missing"
)

type Options struct {
	TrackUIDMap           map[uint64]uint64
	ChapterUIDMap         map[uint64]uint64
	EditionUIDMap         map[uint64]uint64
	Timeline              *timepoint.TimelineMap
	CopyMissingChapters   bool
	MigrateLanguage       bool
	MigrateDefaultFlags   bool
	ClearCompetingDefault bool
	KeepRawUnmappedTimes  bool
}

type StringChange struct {
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Changed bool   `json:"changed"`
}

type BoolChange struct {
	From    bool `json:"from"`
	To      bool `json:"to"`
	Changed bool `json:"changed"`
}

type TrackMigration struct {
	OldUID       uint64       `json:"old_uid,omitempty"`
	NewUID       uint64       `json:"new_uid,omitempty"`
	OldRef       Ref          `json:"old_ref"`
	NewRef       Ref          `json:"new_ref,omitempty"`
	Basis        MatchBasis   `json:"basis"`
	OldLine      uint64       `json:"old_number,omitempty"`
	NewLine      uint64       `json:"new_number,omitempty"`
	Language     StringChange `json:"language,omitempty"`
	LanguageIETF StringChange `json:"language_ietf,omitempty"`
	Default      BoolChange   `json:"default,omitempty"`
}

type RefMigration struct {
	Type    string     `json:"type"`
	FromUID uint64     `json:"from_uid"`
	ToUID   uint64     `json:"to_uid,omitempty"`
	FromRef Ref        `json:"from_ref"`
	ToRef   Ref        `json:"to_ref,omitempty"`
	Basis   MatchBasis `json:"basis"`
	Status  string     `json:"status"`
}

type ChapterMigration struct {
	OldUID   uint64           `json:"old_uid,omitempty"`
	NewUID   uint64           `json:"new_uid,omitempty"`
	OldRef   Ref              `json:"old_ref"`
	NewRef   Ref              `json:"new_ref,omitempty"`
	Basis    MatchBasis       `json:"basis"`
	Start    timepoint.Point  `json:"start"`
	End      *timepoint.Point `json:"end,omitempty"`
	Refs     []RefMigration   `json:"refs,omitempty"`
	Migrated Chapter          `json:"migrated"`
}

type TrackPatch struct {
	TargetUID       uint64 `json:"target_uid"`
	Language        string `json:"language,omitempty"`
	SetLanguage     bool   `json:"set_language,omitempty"`
	LanguageIETF    string `json:"language_ietf,omitempty"`
	SetLanguageIETF bool   `json:"set_language_ietf,omitempty"`
	Default         bool   `json:"default,omitempty"`
	SetDefault      bool   `json:"set_default,omitempty"`
}

type Patch struct {
	TrackPatches    []TrackPatch      `json:"track_patches,omitempty"`
	TrackRefMap     map[uint64]uint64 `json:"track_ref_map,omitempty"`
	Chapters        []Chapter         `json:"chapters,omitempty"`
	ReplaceChapters bool              `json:"replace_chapters,omitempty"`
}

type Report struct {
	SourcePath      string             `json:"source_path,omitempty"`
	TargetPath      string             `json:"target_path,omitempty"`
	Tracks          []TrackMigration   `json:"tracks"`
	Chapters        []ChapterMigration `json:"chapters"`
	Issues          []Issue            `json:"issues,omitempty"`
	Patch           Patch              `json:"patch"`
	UnresolvedTimes []Issue            `json:"unresolved_times,omitempty"`
	UnresolvedRefs  []Issue            `json:"unresolved_refs,omitempty"`
}

func Migrate(source, target *Inventory, opts Options) *Report {
	report := &Report{SourcePath: source.Path, TargetPath: target.Path}
	trackMatch := matchTracks(source, target, opts, report)
	chapterMatch := matchChapters(source, target, opts, report)
	report.Patch.TrackRefMap = map[uint64]uint64{}
	for oldUID, newUID := range trackMatch.uid {
		if oldUID != 0 && newUID != 0 && oldUID != newUID {
			report.Patch.TrackRefMap[oldUID] = newUID
		}
	}
	buildTrackPatches(source, target, opts, trackMatch, report)
	buildChapterMigrations(source, opts, trackMatch, chapterMatch, report)
	if opts.ClearCompetingDefault {
		clearCompetingDefaults(target, trackMatch, report)
	}
	ClassifyReport(report)
	return report
}

type trackMatchResult struct {
	basis       map[int]MatchBasis
	uid         map[uint64]uint64
	targetIndex map[int]int
}

func matchTracks(source, target *Inventory, opts Options, report *Report) trackMatchResult {
	result := trackMatchResult{
		basis: map[int]MatchBasis{}, uid: map[uint64]uint64{}, targetIndex: map[int]int{},
	}
	byUID := map[uint64]int{}
	byFingerprint := map[string][]int{}
	for i, track := range target.Tracks {
		if track.UID != 0 {
			byUID[track.UID] = i
		}
		if track.Fingerprint != "" {
			byFingerprint[track.Fingerprint] = append(byFingerprint[track.Fingerprint], i)
		}
	}
	usedTarget := map[uint64]bool{}

	for i, oldTrack := range source.Tracks {
		migration := TrackMigration{
			OldUID: oldTrack.UID, OldRef: oldTrack.StableRef(), OldLine: oldTrack.Number,
		}
		newIndex := -1
		basis := BasisMissing
		explicitUID, explicit := opts.TrackUIDMap[oldTrack.UID]

		if explicit && oldTrack.UID != 0 {
			if idx, exists := byUID[explicitUID]; exists {
				newIndex, basis = idx, BasisExplicit
			}
		} else if oldTrack.UID != 0 {
			if idx, exists := byUID[oldTrack.UID]; exists {
				newIndex, basis = idx, BasisUID
			}
		}

		if newIndex < 0 && !explicit && oldTrack.Fingerprint != "" {
			if candidates := byFingerprint[oldTrack.Fingerprint]; len(candidates) == 1 {
				newIndex, basis = candidates[0], BasisFingerprint
			} else if len(candidates) > 1 {
				basis = BasisAmbiguous
				report.Issues = append(report.Issues, Issue{
					Code: "track_identity_ambiguous", Severity: SeverityUnresolved,
					Subject: oldTrack.StableRef(),
					Message: fmt.Sprintf("%d target tracks share fingerprint %s; add an explicit UID map",
						len(candidates), oldTrack.Fingerprint),
				})
			}
		}

		if newIndex >= 0 {
			newTrack := target.Tracks[newIndex]
			if usedTarget[newTrack.UID] {
				report.Issues = append(report.Issues, Issue{
					Code: "track_map_not_one_to_one", Severity: SeverityError,
					Subject: oldTrack.StableRef(),
					Message: fmt.Sprintf("target TrackUID %d matched more than once", newTrack.UID),
				})
			}
			usedTarget[newTrack.UID] = true
			result.basis[i] = basis
			result.uid[oldTrack.UID] = newTrack.UID
			result.targetIndex[i] = newIndex
			migration.NewUID, migration.NewRef, migration.Basis = newTrack.UID, newTrack.StableRef(), basis
			migration.NewLine = newTrack.Number
		} else if explicit && oldTrack.UID != 0 {
			result.uid[oldTrack.UID] = explicitUID
			migration.NewUID, migration.NewRef, migration.Basis = explicitUID, TrackRef(explicitUID), BasisExplicit
			report.Issues = append(report.Issues, Issue{
				Code: "track_explicit_target_missing", Severity: SeverityError,
				Subject: oldTrack.StableRef(),
				Message: fmt.Sprintf("explicit target TrackUID %d is absent; references will use it but metadata patch cannot apply", explicitUID),
			})
		} else {
			migration.Basis = basis
			report.Issues = append(report.Issues, Issue{
				Code: "track_identity_missing", Severity: SeverityError,
				Subject: oldTrack.StableRef(),
				Message: "no unique target track identity; references cannot be safely rewritten",
			})
		}
		report.Tracks = append(report.Tracks, migration)
	}
	return result
}

type chapterMatchResult struct {
	uid map[uint64]uint64
}

func matchChapters(source, target *Inventory, opts Options, report *Report) chapterMatchResult {
	result := chapterMatchResult{uid: map[uint64]uint64{}}
	byUID := map[uint64]Chapter{}
	byString := map[string]Chapter{}
	for _, chapter := range target.Chapters {
		if chapter.UID != 0 {
			byUID[chapter.UID] = chapter
		}
		if chapter.StringUID != "" {
			byString[chapter.StringUID] = chapter
		}
	}
	for _, oldChapter := range source.Chapters {
		newUID := uint64(0)
		basis := BasisMissing
		if mapped, ok := opts.ChapterUIDMap[oldChapter.UID]; ok && oldChapter.UID != 0 && mapped != 0 {
			newUID, basis = mapped, BasisExplicit
		} else if oldChapter.UID != 0 {
			if _, exists := byUID[oldChapter.UID]; exists {
				newUID, basis = oldChapter.UID, BasisUID
			}
		}
		if newUID == 0 && oldChapter.StringUID != "" {
			if chapter, exists := byString[oldChapter.StringUID]; exists {
				newUID, basis = chapter.UID, BasisStringUID
			}
		}
		if newUID == 0 && opts.CopyMissingChapters && oldChapter.UID != 0 {
			newUID, basis = oldChapter.UID, BasisCopied
			result.uid[oldChapter.UID] = newUID
		} else if newUID != 0 {
			result.uid[oldChapter.UID] = newUID
		} else {
			report.Issues = append(report.Issues, Issue{
				Code: "chapter_identity_missing", Severity: SeverityWarning,
				Subject: ChapterRef(oldChapter.UID),
				Message: "chapter has no target identity; it will only appear in the report",
			})
		}
		report.Chapters = append(report.Chapters, ChapterMigration{
			OldUID: oldChapter.UID, NewUID: newUID, OldRef: ChapterRef(oldChapter.UID),
			NewRef: refIfChapter(newUID), Basis: basis, Start: oldChapter.Start, End: oldChapter.End,
		})
	}
	return result
}

func refIfChapter(uid uint64) Ref {
	if uid == 0 {
		return ""
	}
	return ChapterRef(uid)
}
