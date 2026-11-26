package identity

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildGraph records every identity/reference in one inventory. Line numbers
// are deliberately absent from edges; Number/LegacyLine remain attributes for
// diagnostics only.
func BuildGraph(inv *Inventory) Graph {
	g := Graph{}
	trackByUID := map[uint64]int{}
	chapterByUID := map[uint64]int{}
	editionByUID := map[uint64]int{}

	for i := range inv.Tracks {
		track := inv.Tracks[i]
		ref := track.StableRef()
		g.Nodes = append(g.Nodes, Node{
			Ref: ref, Kind: KindTrack, UID: track.UID,
			StableID: track.Fingerprint, Label: track.Label(),
		})
		if track.UID == 0 {
			g.Issues = append(g.Issues, Issue{
				Code: "track_uid_missing", Severity: SeverityError, Subject: ref,
				Message: fmt.Sprintf("track number %d has no TrackUID; identity is weak", track.Number),
			})
		} else if prev, ok := trackByUID[track.UID]; ok {
			g.Issues = append(g.Issues, Issue{
				Code: "track_uid_duplicate", Severity: SeverityError, Subject: ref,
				Message: fmt.Sprintf("TrackUID duplicates tracks at indexes %d and %d", prev, i),
			})
		} else {
			trackByUID[track.UID] = i
		}
	}

	for i := range inv.Editions {
		edition := inv.Editions[i]
		ref := EditionRef(edition.UID)
		g.Nodes = append(g.Nodes, Node{Ref: ref, Kind: KindEdition, UID: edition.UID})
		if _, dup := editionByUID[edition.UID]; dup {
			g.Issues = append(g.Issues, Issue{
				Code: "edition_uid_duplicate", Severity: SeverityError, Subject: ref,
				Message: "duplicate EditionUID",
			})
		}
		editionByUID[edition.UID] = i
		for _, chapterUID := range edition.ChapterUIDs {
			g.Edges = append(g.Edges, Edge{From: ref, To: ChapterRef(chapterUID), Type: "edition-contains"})
		}
	}

	for i := range inv.Chapters {
		chapter := inv.Chapters[i]
		ref := ChapterRef(chapter.UID)
		label := ""
		if len(chapter.Displays) > 0 {
			label = chapter.Displays[0].String
		}
		g.Nodes = append(g.Nodes, Node{Ref: ref, Kind: KindChapter, UID: chapter.UID, Label: label})
		if chapter.UID == 0 {
			g.Issues = append(g.Issues, Issue{
				Code: "chapter_uid_missing", Severity: SeverityError, Subject: ref,
				Message: "chapter has no ChapterUID; use StringUID or an explicit identity map",
			})
		} else if _, dup := chapterByUID[chapter.UID]; dup {
			g.Issues = append(g.Issues, Issue{
				Code: "chapter_uid_duplicate", Severity: SeverityError, Subject: ref,
				Message: "duplicate ChapterUID",
			})
		} else {
			chapterByUID[chapter.UID] = i
		}

		if !chapter.Start.Ok() {
			g.Issues = append(g.Issues, Issue{
				Code: "chapter_start_unresolved", Severity: SeverityUnresolved,
				Subject: ref, Field: "ChapterTimeStart", Message: chapter.Start.Reason,
			})
		}
		if chapter.End != nil && !chapter.End.Ok() {
			g.Issues = append(g.Issues, Issue{
				Code: "chapter_end_unresolved", Severity: SeverityUnresolved,
				Subject: ref, Field: "ChapterTimeEnd", Message: chapter.End.Reason,
			})
		}
		if chapter.Start.Ok() && chapter.End != nil && chapter.End.Ok() &&
			*chapter.End.NS < *chapter.Start.NS {
			g.Issues = append(g.Issues, Issue{
				Code: "chapter_end_before_start", Severity: SeverityError, Subject: ref,
				Message: "ChapterTimeEnd is earlier than ChapterTimeStart",
			})
		}

		if chapter.ParentUID != 0 {
			g.Edges = append(g.Edges, Edge{From: ref, To: ChapterRef(chapter.ParentUID), Type: "chapter-parent"})
		}
		if chapter.EditionUID != 0 {
			g.Edges = append(g.Edges, Edge{From: ref, To: EditionRef(chapter.EditionUID), Type: "chapter-edition"})
		}
		for _, trackUID := range chapter.TrackUIDs {
			g.Edges = append(g.Edges, Edge{
				From: ref, To: TrackRef(trackUID), Type: "chapter-track",
				Context: "ChapterTrackUID",
			})
		}
	}

	for _, edge := range g.Edges {
		if edge.Type != "chapter-track" {
			continue
		}
		uid := refUID(edge.To)
		if _, ok := trackByUID[uid]; !ok {
			g.Issues = append(g.Issues, Issue{
				Code: "dangling_chapter_track_ref", Severity: SeverityError,
				Subject: edge.From, Message: fmt.Sprintf("chapter references missing TrackUID %d", uid),
			})
		}
	}
	for _, edge := range g.Edges {
		if edge.Type == "chapter-parent" || edge.Type == "edition-contains" {
			uid := refUID(edge.To)
			if _, ok := chapterByUID[uid]; !ok {
				g.Issues = append(g.Issues, Issue{
					Code: "dangling_chapter_ref", Severity: SeverityError,
					Subject: edge.From, Message: fmt.Sprintf("%s points to missing chapter UID %d", edge.Type, uid),
				})
			}
		}
	}

	g.Issues = append(g.Issues, defaultTrackIssues(inv)...)
	return g
}

func refUID(ref Ref) uint64 {
	raw := string(ref)
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		raw = raw[i+1:]
	}
	uid, _ := strconv.ParseUint(raw, 16, 64)
	return uid
}

func (t Track) StableRef() Ref {
	if t.UID != 0 {
		return TrackRef(t.UID)
	}
	if t.Fingerprint != "" {
		return Ref("track:fp:" + t.Fingerprint)
	}
	if t.LegacyLine > 0 {
		return Ref(fmt.Sprintf("track:legacy-line:%d", t.LegacyLine))
	}
	return Ref(fmt.Sprintf("track:number:%d", t.Number))
}

func (t Track) Label() string {
	switch {
	case t.Name != "":
		return t.Name
	case t.LanguageIETF != "":
		return t.LanguageIETF
	case t.Language != "":
		return t.Language
	default:
		return t.CodecID
	}
}

func defaultTrackIssues(inv *Inventory) []Issue {
	var issues []Issue
	byType := map[TrackType][]Track{}
	for _, track := range inv.Tracks {
		if effectiveDefault(track) {
			byType[track.Type] = append(byType[track.Type], track)
		}
	}
	for typ, tracks := range byType {
		if len(tracks) > 1 {
			refs := make([]Ref, 0, len(tracks))
			for _, track := range tracks {
				refs = append(refs, track.StableRef())
			}
			issues = append(issues, Issue{
				Code: "multiple_default_tracks", Severity: SeverityWarning,
				Message: fmt.Sprintf("track type %d has %d default tracks: %v", typ, len(tracks), refs),
			})
		}
	}
	return issues
}
