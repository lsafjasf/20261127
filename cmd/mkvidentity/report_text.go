package main

import (
	"fmt"
	"strings"

	"mkvidentity/identity"
)

func printInventoryText(inv *identity.Inventory, graph *identity.Graph) {
	fmt.Printf("File: %s\n", orDash(inv.Path))
	fmt.Printf("Tracks: %d  Editions: %d  Chapters: %d\n", len(inv.Tracks), len(inv.Editions), len(inv.Chapters))
	for _, track := range inv.Tracks {
		flags := make([]string, 0, 3)
		if track.Default {
			flags = append(flags, "default")
		}
		if track.Forced {
			flags = append(flags, "forced")
		}
		if !track.Enabled {
			flags = append(flags, "disabled")
		}
		fmt.Printf("  T uid=%d number=%d type=%d lang=%s ietf=%s [%s] %s\n",
			track.UID, track.Number, track.Type, orDash(track.Language), orDash(track.LanguageIETF),
			strings.Join(flags, ","), orDash(track.Name))
	}
	for _, chapter := range inv.Chapters {
		title := ""
		if len(chapter.Displays) > 0 {
			title = chapter.Displays[0].String
		}
		start := "UNRESOLVED"
		if chapter.Start.Ok() {
			start = chapter.Start.Raw
		}
		fmt.Printf("  C uid=%d line=%d start=%s tracks=%v %q\n",
			chapter.UID, chapter.LegacyLine, start, chapter.TrackUIDs, title)
	}
	printGraphText(graph)
}

func printGraphText(graph *identity.Graph) {
	if len(graph.Issues) == 0 {
		fmt.Println("Issues: none")
		return
	}
	fmt.Printf("Issues: %d\n", len(graph.Issues))
	for _, issue := range graph.Issues {
		subject := ""
		if issue.Subject != "" {
			subject = " " + string(issue.Subject)
		}
		fmt.Printf("  [%s] %s%s: %s\n", issue.Severity, issue.Code, subject, issue.Message)
	}
}

func printMigrationText(report *identity.Report) {
	fmt.Printf("Track migrations: %d\n", len(report.Tracks))
	for _, tm := range report.Tracks {
		fmt.Printf("  %s -> %s (%s, number %d->%d)\n",
			tm.OldRef, tm.NewRef, tm.Basis, tm.OldLine, tm.NewLine)
	}
	fmt.Printf("Chapter migrations: %d\n", len(report.Chapters))
	for _, cm := range report.Chapters {
		start := "UNRESOLVED"
		if cm.Start.Ok() {
			start = cm.Start.Raw
		}
		fmt.Printf("  %s -> %s (%s) start=%s refs=%d\n",
			cm.OldRef, cm.NewRef, cm.Basis, start, len(cm.Refs))
	}
	if len(report.UnresolvedTimes) > 0 {
		fmt.Printf("Unresolved time points: %d\n", len(report.UnresolvedTimes))
		for _, issue := range report.UnresolvedTimes {
			fmt.Printf("  [%s] %s %s: %s\n", issue.Severity, issue.Code, issue.Subject, issue.Message)
		}
	}
	printGraphText(&identity.Graph{Issues: report.Issues})
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
