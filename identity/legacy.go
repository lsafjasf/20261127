package identity

import (
	"fmt"

	"mkvidentity/timepoint"
)

// LegacyTrackEdit is the line-oriented input accepted from an old script.
// ApplyLegacyPlan resolves Line to a stable TrackUID before migration.
type LegacyTrackEdit struct {
	Line         int    `json:"line"`
	Language     string `json:"language,omitempty"`
	LanguageIETF string `json:"language_ietf,omitempty"`
	Name         string `json:"name,omitempty"`
	Default      *bool  `json:"default,omitempty"`
}

// LegacyChapterEdit uses line numbers only for compatibility. TrackLines are
// resolved to TrackUIDs immediately; bad time strings become unresolved Points.
type LegacyChapterEdit struct {
	Line       int    `json:"line"`
	Start      string `json:"start,omitempty"`
	End        string `json:"end,omitempty"`
	TrackLines []int  `json:"track_lines,omitempty"`
}

type LegacyPlan struct {
	FPS      string              `json:"fps,omitempty"`
	Tracks   []LegacyTrackEdit   `json:"tracks,omitempty"`
	Chapters []LegacyChapterEdit `json:"chapters,omitempty"`
}

func ApplyLegacyPlan(inv *Inventory, plan LegacyPlan) []Issue {
	var issues []Issue
	opts := timepoint.ParseOptions{}
	if plan.FPS != "" {
		rate, err := timepoint.ParseRate(plan.FPS)
		if err != nil {
			issues = append(issues, Issue{
				Code: "invalid_fps", Severity: SeverityError, Message: err.Error(),
			})
		} else {
			opts.FPS = rate
		}
	}

	for _, edit := range plan.Tracks {
		index, basis, err := trackIndexForLine(inv, edit.Line)
		if err != nil {
			issues = append(issues, Issue{
				Code: "legacy_track_line_unresolved", Severity: SeverityError,
				Message: err.Error(),
			})
			continue
		}
		if basis == "track_number" {
			issues = append(issues, Issue{
				Code: "legacy_line_used_track_number", Severity: SeverityWarning,
				Subject: inv.Tracks[index].StableRef(),
				Message: fmt.Sprintf("line %d did not match entry order; used TrackNumber", edit.Line),
			})
		}
		track := &inv.Tracks[index]
		if edit.Language != "" {
			track.Language = edit.Language
		}
		if edit.LanguageIETF != "" {
			track.LanguageIETF = edit.LanguageIETF
		}
		if edit.Name != "" {
			track.Name = edit.Name
		}
		if edit.Default != nil {
			track.DefaultSet, track.Default = true, *edit.Default
		}
	}

	for _, edit := range plan.Chapters {
		index, err := chapterIndexForLine(inv, edit.Line)
		if err != nil {
			issues = append(issues, Issue{
				Code: "legacy_chapter_line_unresolved", Severity: SeverityError,
				Message: err.Error(),
			})
			continue
		}
		chapter := &inv.Chapters[index]
		if edit.Start != "" {
			chapter.Start = timepoint.Parse(edit.Start, opts)
			if !chapter.Start.Ok() {
				issues = append(issues, Issue{
					Code: "chapter_start_unresolved", Severity: SeverityUnresolved,
					Subject: ChapterRef(chapter.UID), Field: "legacy.start",
					Message: chapter.Start.Reason,
				})
			}
		}
		if edit.End != "" {
			end := timepoint.Parse(edit.End, opts)
			chapter.End = &end
			if !end.Ok() {
				issues = append(issues, Issue{
					Code: "chapter_end_unresolved", Severity: SeverityUnresolved,
					Subject: ChapterRef(chapter.UID), Field: "legacy.end", Message: end.Reason,
				})
			}
		}
		if len(edit.TrackLines) > 0 {
			chapter.TrackUIDs = nil
			for _, line := range edit.TrackLines {
				trackIndex, _, err := trackIndexForLine(inv, line)
				if err != nil {
					issues = append(issues, Issue{
						Code: "legacy_track_ref_unresolved", Severity: SeverityUnresolved,
						Subject: ChapterRef(chapter.UID),
						Message: fmt.Sprintf("chapter line %d references unresolved track line %d", edit.Line, line),
					})
					continue
				}
				uid := inv.Tracks[trackIndex].UID
				if uid == 0 {
					issues = append(issues, Issue{
						Code: "track_uid_missing", Severity: SeverityError,
						Subject: inv.Tracks[trackIndex].StableRef(),
						Message: fmt.Sprintf("track line %d has no UID", line),
					})
					continue
				}
				chapter.TrackUIDs = append(chapter.TrackUIDs, uid)
			}
		}
	}
	return issues
}

func trackIndexForLine(inv *Inventory, line int) (int, string, error) {
	if line <= 0 {
		return -1, "", fmt.Errorf("track line %d is invalid", line)
	}
	for i := range inv.Tracks {
		if inv.Tracks[i].LegacyLine == line {
			return i, "entry_order", nil
		}
	}
	for i := range inv.Tracks {
		if inv.Tracks[i].Number == uint64(line) {
			return i, "track_number", nil
		}
	}
	return -1, "", fmt.Errorf("track line %d does not resolve to any track identity", line)
}

func chapterIndexForLine(inv *Inventory, line int) (int, error) {
	if line <= 0 {
		return -1, fmt.Errorf("chapter line %d is invalid", line)
	}
	for i := range inv.Chapters {
		if inv.Chapters[i].LegacyLine == line {
			return i, nil
		}
	}
	return -1, fmt.Errorf("chapter line %d does not resolve to any chapter identity", line)
}
