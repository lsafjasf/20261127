package identity

import (
	"testing"

	"mkvidentity/timepoint"
)

func TestLegacyLinesResolveToUIDs(t *testing.T) {
	inv := &Inventory{
		Tracks: []Track{
			{UID: 101, Number: 9, LegacyLine: 1, Type: TrackVideo},
			{UID: 201, Number: 2, LegacyLine: 2, Type: TrackAudio},
		},
		Chapters: []Chapter{{UID: 700, LegacyLine: 1, Start: timepoint.Resolved(0)}},
	}
	isDefault := true
	plan := LegacyPlan{
		Tracks:   []LegacyTrackEdit{{Line: 2, Language: "yue", Default: &isDefault}},
		Chapters: []LegacyChapterEdit{{Line: 1, TrackLines: []int{2}}},
	}
	issues := ApplyLegacyPlan(inv, plan)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if inv.Tracks[1].Language != "yue" || !inv.Tracks[1].DefaultSet || !inv.Tracks[1].Default {
		t.Fatalf("line edit not resolved to UID track: %+v", inv.Tracks[1])
	}
	if got := inv.Chapters[0].TrackUIDs; len(got) != 1 || got[0] != 201 {
		t.Fatalf("chapter line ref = %v, want [201]", got)
	}

	plan.Tracks[0].Line = 99
	issues = ApplyLegacyPlan(inv, plan)
	if len(issues) == 0 || issues[0].Code != "legacy_track_line_unresolved" {
		t.Fatalf("missing line resolution issue: %+v", issues)
	}
}
