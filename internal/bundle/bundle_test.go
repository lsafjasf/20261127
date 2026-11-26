package bundle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// png1x1 is a structurally valid 1x1 PNG so media sniffing has real
// binary signatures to classify rather than plain text.
var png1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, png1x1, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReproduciblePackTarAndGzip(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"b.txt":     "second",
		"dir/a.txt": "first",
		"c.json":    `{"k":"v"}`,
	})
	writePNG(t, filepath.Join(root, "img", "pixel.png"))

	dir := t.TempDir()
	for _, comp := range []string{"", "gzip"} {
		out1 := filepath.Join(dir, "one")
		out2 := filepath.Join(dir, "two")
		m1, err := Pack(root, out1, Options{Compression: comp, SourceRoot: root})
		if err != nil {
			t.Fatalf("pack 1 (%q): %v", comp, err)
		}
		m2, err := Pack(root, out2, Options{Compression: comp, SourceRoot: root})
		if err != nil {
			t.Fatalf("pack 2 (%q): %v", comp, err)
		}
		h1, _ := HashFile(out1)
		h2, _ := HashFile(out2)
		if h1 != h2 {
			t.Fatalf("compression=%q: hashes differ across runs: %s vs %s", comp, h1, h2)
		}
		if m1.BundleID != m2.BundleID {
			t.Fatalf("bundle id drifted: %s vs %s", m1.BundleID, m2.BundleID)
		}

		pb, err := ReadBundle(out1)
		if err != nil {
			t.Fatal(err)
		}
		wantOrder := []string{
			BundleManifestPath, "b.txt", "c.json", "dir/a.txt", "img/pixel.png",
		}
		if !equalStrings(pb.EntryOrder, wantOrder) {
			t.Fatalf("container order = %v, want %v", pb.EntryOrder, wantOrder)
		}
		if got := pb.Entries["img/pixel.png"].MediaKind; got != KindImage {
			t.Fatalf("media kind = %q, want image", got)
		}
	}
}

func TestIdenticalBundlesCompareIdentical(t *testing.T) {
	root, dir := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{"a": "hello"})
	out1, out2 := filepath.Join(dir, "1.tar"), filepath.Join(dir, "2.tar")
	if _, err := Pack(root, out1, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(root, out2, Options{}); err != nil {
		t.Fatal(err)
	}
	report, err := Compare(out1, out2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictIdentical || !report.RawIdentical {
		t.Fatalf("verdict=%s raw=%t", report.Verdict, report.RawIdentical)
	}
}

func TestLegacyBundlesAreCosmeticEquivalent(t *testing.T) {
	root, dir := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{
		"z": "1", "y": "2", "x": "3", "w": "4", "v": "5",
	})
	writePNG(t, filepath.Join(root, "p.png"))

	out1, out2 := filepath.Join(dir, "1.tar.gz"), filepath.Join(dir, "2.tar.gz")
	if _, err := LegacyPack(root, out1, Options{Compression: "gzip"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // force a different packedAt second
	if _, err := LegacyPack(root, out2, Options{Compression: "gzip"}); err != nil {
		t.Fatal(err)
	}

	h1, _ := HashFile(out1)
	h2, _ := HashFile(out2)
	if h1 == h2 {
		t.Fatal("legacy packs unexpectedly identical")
	}

	report, err := Compare(out1, out2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictCosmeticEquivalent {
		t.Fatalf("verdict = %q, want %q; report=%+v",
			report.Verdict, VerdictCosmeticEquivalent, report)
	}
	if !report.BundleIDDifference {
		t.Fatal("random bundle id difference not detected")
	}
	if len(report.PackagingTimestamps) == 0 {
		t.Fatal("packaging timestamp difference not detected")
	}
	if !report.MediaEquivalent {
		t.Fatal("media should be equivalent for unchanged inputs")
	}
}

func TestRealMediaDifferenceIsDetected(t *testing.T) {
	root1, root2, dir := t.TempDir(), t.TempDir(), t.TempDir()
	writeTree(t, root1, map[string]string{"a": "one", "shared": "same"})
	writeTree(t, root2, map[string]string{"a": "two", "shared": "same"})
	out1, out2 := filepath.Join(dir, "1.tar"), filepath.Join(dir, "2.tar")
	if _, err := Pack(root1, out1, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(root2, out2, Options{}); err != nil {
		t.Fatal(err)
	}
	report, err := Compare(out1, out2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictMediaDifferent {
		t.Fatalf("verdict = %q, want media-different", report.Verdict)
	}
	if len(report.MediaDiffs) != 1 || report.MediaDiffs[0].Path != "a" {
		t.Fatalf("media diffs = %+v", report.MediaDiffs)
	}
}

func TestAddedRemovedAndMetadataDiffs(t *testing.T) {
	root1, root2, dir := t.TempDir(), t.TempDir(), t.TempDir()
	writeTree(t, root1, map[string]string{"keep": "same", "old": "gone"})
	writeTree(t, root2, map[string]string{"keep": "same", "new": "here"})

	// Make the shared file's mtime differ while bytes stay identical.
	future := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(root2, "keep"), future, future); err != nil {
		t.Fatal(err)
	}

	out1, out2 := filepath.Join(dir, "1.tar"), filepath.Join(dir, "2.tar")
	if _, err := Pack(root1, out1, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(root2, out2, Options{}); err != nil {
		t.Fatal(err)
	}
	report, err := Compare(out1, out2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictMediaDifferent {
		t.Fatalf("verdict = %q", report.Verdict)
	}
	kinds := map[string]bool{}
	for _, d := range report.MediaDiffs {
		kinds[d.Path+":"+d.Kind] = true
	}
	if !kinds["old:removed-in-b"] || !kinds["new:added-in-b"] {
		t.Fatalf("add/remove not classified: %+v", report.MediaDiffs)
	}
	var sawMtime bool
	for _, d := range report.SourceMetadataDiffs {
		if d.Path == "keep" && d.Field == "modTime" {
			sawMtime = true
		}
	}
	if !sawMtime {
		t.Fatalf("source mtime difference not reported: %+v", report.SourceMetadataDiffs)
	}

	// Trees with identical bytes but different mtimes only -> metadata verdict.
	root3, root4, dir2 := t.TempDir(), t.TempDir(), t.TempDir()
	writeTree(t, root3, map[string]string{"keep": "same"})
	writeTree(t, root4, map[string]string{"keep": "same"})
	if err := os.Chtimes(filepath.Join(root4, "keep"), future, future); err != nil {
		t.Fatal(err)
	}
	o1, o2 := filepath.Join(dir2, "1.tar"), filepath.Join(dir2, "2.tar")
	_, _ = Pack(root3, o1, Options{})
	_, _ = Pack(root4, o2, Options{})
	metaReport, err := Compare(o1, o2)
	if err != nil {
		t.Fatal(err)
	}
	if metaReport.Verdict != VerdictMetadataDifferent {
		t.Fatalf("verdict = %q, want metadata-different", metaReport.Verdict)
	}
}

func TestCanonicalIDIsContentDerived(t *testing.T) {
	root1, root2 := t.TempDir(), t.TempDir()
	files := map[string]string{"a": "x", "dir/b": "y"}
	writeTree(t, root1, files)
	writeTree(t, root2, files)
	e1, err := Snapshot(root1)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := Snapshot(root2)
	if err != nil {
		t.Fatal(err)
	}
	// Normalise source mtimes: this test asserts content-derived identity,
	// and writeTree creates each tree with its own creation timestamps.
	zeroMtimes(e1)
	zeroMtimes(e2)
	if DeriveBundleID(e1) != DeriveBundleID(e2) {
		t.Fatal("identical content produced different bundle ids")
	}

	writeTree(t, root2, map[string]string{"a": "changed"})
	e3, _ := Snapshot(root2)
	zeroMtimes(e3)
	if DeriveBundleID(e1) == DeriveBundleID(e3) {
		t.Fatal("content change did not change bundle id")
	}
}

func zeroMtimes(entries []*Entry) {
	for _, e := range entries {
		e.ModTime = time.Unix(0, 0).UTC()
	}
}

func TestPortableAcrossCheckoutPathsAndCompression(t *testing.T) {
	root1, root2, dir := t.TempDir(), t.TempDir(), t.TempDir()
	files := map[string]string{"a": "x", "nested/b": "yy"}
	writeTree(t, root1, files)
	writeTree(t, root2, files)

	for _, comp := range []string{"", "gzip"} {
		o1 := filepath.Join(dir, "1")
		o2 := filepath.Join(dir, "2")
		if _, err := Pack(root1, o1, Options{Compression: comp}); err != nil {
			t.Fatal(err)
		}
		if _, err := Pack(root2, o2, Options{Compression: comp}); err != nil {
			t.Fatal(err)
		}
		h1, _ := HashFile(o1)
		h2, _ := HashFile(o2)
		if h1 != h2 {
			t.Fatalf("compression=%q: checkout path leaked into bytes", comp)
		}
	}

	// Explicit provenance labels are retained but, when they differ, the
	// media-equivalence verdict still holds because BundleID ignores them.
	o1 := filepath.Join(dir, "label1.tar")
	o2 := filepath.Join(dir, "label2.tar")
	if _, err := Pack(root1, o1, Options{SourceRoot: "build-A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(root2, o2, Options{SourceRoot: "build-B"}); err != nil {
		t.Fatal(err)
	}
	report, err := Compare(o1, o2)
	if err != nil {
		t.Fatal(err)
	}
	if !report.MediaEquivalent || len(report.MediaDiffs) != 0 {
		t.Fatalf("different provenance labels must not count as media diffs: %+v", report)
	}
}

func TestModeDifferenceIsSourceMetadata(t *testing.T) {
	root1, root2, dir := t.TempDir(), t.TempDir(), t.TempDir()
	writeTree(t, root1, map[string]string{"script": "#!/bin/sh\n"})
	writeTree(t, root2, map[string]string{"script": "#!/bin/sh\n"})
	if err := os.Chmod(filepath.Join(root1, "script"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root2, "script"), 0o755); err != nil {
		t.Fatal(err)
	}
	o1, o2 := filepath.Join(dir, "1.tar"), filepath.Join(dir, "2.tar")
	if _, err := Pack(root1, o1, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(root2, o2, Options{}); err != nil {
		t.Fatal(err)
	}
	report, err := Compare(o1, o2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictMetadataDifferent {
		t.Fatalf("verdict = %q, want metadata-different", report.Verdict)
	}
	var sawMode bool
	for _, d := range report.SourceMetadataDiffs {
		if d.Field == "mode" && d.A == "0644" && d.B == "0755" {
			sawMode = true
		}
	}
	if !sawMode {
		t.Fatalf("mode diff missing: %+v", report.SourceMetadataDiffs)
	}
}

func TestCanonicalAndLegacyBundlesWithSameInputAreEquivalent(t *testing.T) {
	root, dir := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{"a": "same", "nested/b": "also"})
	can := filepath.Join(dir, "can.tar")
	leg := filepath.Join(dir, "leg.tar.gz")
	if _, err := Pack(root, can, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := LegacyPack(root, leg, Options{Compression: "gzip"}); err != nil {
		t.Fatal(err)
	}
	report, err := Compare(can, leg)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictCosmeticEquivalent {
		t.Fatalf("verdict = %q, want cosmetic-equivalent; report=%+v",
			report.Verdict, report)
	}
	if !report.MediaEquivalent {
		t.Fatal("same input must be media-equivalent across layouts")
	}
}
