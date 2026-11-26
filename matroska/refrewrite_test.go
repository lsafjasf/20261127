package matroska

import (
	"os"
	"path/filepath"
	"testing"

	"mkvidentity/identity"
)

func TestRewriteExistingChapterTrackRefsByUIDMap(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.mkv")
	if err := os.WriteFile(sourcePath, buildTestMKV(t), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "refs.mkv")
	patch := identity.Patch{TrackRefMap: map[uint64]uint64{101: 102}}
	result, err := RewriteFile(sourcePath, outPath, patch)
	if err != nil {
		t.Fatal(err)
	}
	if result.ChapterRefsRewritten != 1 {
		t.Fatalf("ChapterRefsRewritten = %d, want 1 (%+v)", result.ChapterRefsRewritten, result)
	}
	inv, err := ReadInventoryPath(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Chapters) != 1 || len(inv.Chapters[0].TrackUIDs) != 1 || inv.Chapters[0].TrackUIDs[0] != 102 {
		t.Fatalf("chapter refs = %+v", inv.Chapters)
	}
	assertIndexTargets(t, outPath)
}
