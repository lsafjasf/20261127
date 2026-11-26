package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"mkvidentity/identity"
	"mkvidentity/matroska"
	"mkvidentity/timepoint"
)

type uidMapFlag map[uint64]uint64

func (m uidMapFlag) String() string {
	parts := make([]string, 0, len(m))
	for oldUID, newUID := range m {
		parts = append(parts, fmt.Sprintf("%d=%d", oldUID, newUID))
	}
	return strings.Join(parts, ",")
}

func (m uidMapFlag) Set(raw string) error {
	parts := strings.SplitN(raw, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("map must be OLD=NEW")
	}
	oldUID, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 0, 64)
	if err != nil {
		return fmt.Errorf("invalid old UID %q: %w", parts[0], err)
	}
	newUID, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 64)
	if err != nil {
		return fmt.Errorf("invalid new UID %q: %w", parts[1], err)
	}
	m[oldUID] = newUID
	return nil
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	oldPath := fs.String("old", "", "old MKV or inventory JSON")
	newPath := fs.String("new", "", "new/remuxed MKV or inventory JSON")
	legacyRulesPath := fs.String("legacy-rules", "", "line-oriented legacy rewrite JSON")
	timelinePath := fs.String("timeline", "", "timeline cut/offset JSON")
	fps := fs.String("fps", "", "frame rate such as 24000/1001 for frame-coded legacy times")
	trackMap := uidMapFlag{}
	chapterMap := uidMapFlag{}
	editionMap := uidMapFlag{}
	fs.Var(trackMap, "map-track", "explicit old=new TrackUID map (repeatable)")
	fs.Var(chapterMap, "map-chapter", "explicit old=new ChapterUID map (repeatable)")
	fs.Var(editionMap, "map-edition", "explicit old=new EditionUID map (repeatable)")
	copyChapters := fs.Bool("copy-chapters", true, "copy/remap chapters when target has none")
	migrateLanguage := fs.Bool("language", true, "migrate language tags")
	migrateDefault := fs.Bool("default-flags", true, "migrate default track flags")
	clearCompeting := fs.Bool("clear-competing-default", true, "clear competing default tracks of same type")
	keepRawTimes := fs.Bool("keep-raw-times", false, "write original EBML times for unmappable points while still reporting them")
	patchOut := fs.String("patch", "", "write identity patch JSON")
	reportOut := fs.String("report", "", "write report JSON")
	outMKV := fs.String("out", "", "rewrite target MKV to this path")
	asJSON := fs.Bool("json", false, "print JSON report")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if *oldPath == "" || *newPath == "" {
		return fmt.Errorf("migrate requires --old and --new")
	}

	oldInv, err := matroska.LoadInventory(*oldPath)
	if err != nil {
		return fmt.Errorf("load old: %w", err)
	}
	newInv, err := matroska.LoadInventory(*newPath)
	if err != nil {
		return fmt.Errorf("load new: %w", err)
	}

	var legacyIssues []identity.Issue
	if *legacyRulesPath != "" {
		plan, err := loadLegacyPlan(*legacyRulesPath)
		if err != nil {
			return err
		}
		if *fps != "" {
			plan.FPS = *fps
		}
		legacyIssues = identity.ApplyLegacyPlan(oldInv, plan)
	}

	opts := identity.Options{
		TrackUIDMap: trackMap, ChapterUIDMap: chapterMap, EditionUIDMap: editionMap,
		CopyMissingChapters: *copyChapters, MigrateLanguage: *migrateLanguage,
		MigrateDefaultFlags: *migrateDefault, ClearCompetingDefault: *clearCompeting,
		KeepRawUnmappedTimes: *keepRawTimes,
	}
	if *timelinePath != "" {
		timeline, err := loadTimeline(*timelinePath)
		if err != nil {
			return err
		}
		opts.Timeline = timeline
	}
	report := identity.Migrate(oldInv, newInv, opts)
	report.Issues = append(legacyIssues, report.Issues...)
	identity.ClassifyReport(report)

	if *patchOut != "" {
		if err := writeJSON(*patchOut, report.Patch); err != nil {
			return err
		}
	}
	var rewriteResult *matroska.RewriteResult
	if *outMKV != "" {
		if report.Patch.ReplaceChapters && !*keepRawTimes && patchHasUnencodableTime(report.Patch) {
			return fmt.Errorf("chapter patch has unresolved time points; inspect --report, fix --timeline, or pass --keep-raw-times")
		}
		targetMKV := *newPath
		if strings.HasSuffix(strings.ToLower(targetMKV), ".json") {
			return fmt.Errorf("--out requires --new to point to the target MKV, not an inventory JSON")
		}
		rewriteResult, err = matroska.RewriteFile(targetMKV, *outMKV, report.Patch)
		if err != nil {
			return fmt.Errorf("rewrite mkv: %w", err)
		}
	}
	if *reportOut != "" {
		envelope := map[string]any{"report": report, "rewrite": rewriteResult}
		if err := writeJSON(*reportOut, envelope); err != nil {
			return err
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"report": report, "rewrite": rewriteResult})
	}
	printMigrationText(report)
	if rewriteResult != nil {
		fmt.Printf("Wrote %s (%d bytes, %d seek entries, %d cue positions fixed)\n",
			rewriteResult.OutputPath, rewriteResult.BytesWritten,
			rewriteResult.SeekEntriesFixed, rewriteResult.CuePositionsFixed)
	}
	return nil
}

func patchHasUnencodableTime(patch identity.Patch) bool {
	for _, chapter := range patch.Chapters {
		if chapter.Start.NS == nil {
			return true
		}
		if chapter.End != nil && chapter.End.NS == nil {
			return true
		}
	}
	return false
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	input := fs.String("input", "", "target MKV path")
	patchPath := fs.String("patch", "", "patch JSON path")
	out := fs.String("out", "", "output MKV path")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if *input == "" || *patchPath == "" || *out == "" {
		return fmt.Errorf("apply requires --input, --patch, and --out")
	}
	var patch identity.Patch
	if err := readJSON(*patchPath, &patch); err != nil {
		return err
	}
	result, err := matroska.RewriteFile(*input, *out, patch)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func loadLegacyPlan(path string) (identity.LegacyPlan, error) {
	var plan identity.LegacyPlan
	err := readJSON(path, &plan)
	return plan, err
}

func loadTimeline(path string) (*timepoint.TimelineMap, error) {
	var timeline timepoint.TimelineMap
	if err := readJSON(path, &timeline); err != nil {
		return nil, err
	}
	normalized, err := timeline.Normalized()
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
