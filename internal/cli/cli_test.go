package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const repoTestdata = "../../testdata"

func TestCLIBuildsReportAndPlaylist(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"-main", filepath.Join(repoTestdata, "main-imp"),
		"-supp", filepath.Join(repoTestdata, "supp-imp"),
		"-out", out,
		"-copy-assets",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("default run should exit 0, got %d\nstderr=%s", code, stderr.String())
	}
	text := stdout.String()
	if !strings.Contains(text, "MainSubtitleSequence") {
		t.Fatalf("stdout report should mention subtitle sequence:\n%s", text)
	}
	if !strings.Contains(text, "unresolved_reference") || !strings.Contains(text, "duplicate_asset_uuid") {
		t.Fatalf("stdout should contain missing/duplicate findings:\n%s", text)
	}
	for _, f := range []string{"playlist.json", "report.json", "report.txt", "CPL_final.xml", "assets/SUBTITLE_5555.mxf"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Fatalf("missing output %s: %v", f, err)
		}
	}
}

func TestCLIStrictExitCode(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"-main", filepath.Join(repoTestdata, "main-imp"),
		"-supp", filepath.Join(repoTestdata, "supp-imp"),
		"-out", out,
		"-strict",
		"-quiet",
	}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("strict run with missing+conflict should exit 3, got %d", code)
	}
}

func TestCLIRejectsOutputInsidePackage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"-main", filepath.Join(repoTestdata, "main-imp"),
		"-out", filepath.Join(repoTestdata, "main-imp", "nested-out"),
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("output inside package should be usage error 2, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(repoTestdata, "main-imp", "nested-out")); !os.IsNotExist(err) {
		os.RemoveAll(filepath.Join(repoTestdata, "main-imp", "nested-out"))
		t.Fatalf("rejected output dir must not be created")
	}
}

func TestCLIPositionalArgs(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		filepath.Join(repoTestdata, "main-imp"),
		filepath.Join(repoTestdata, "supp-imp"),
		"-out", out,
		"-quiet",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("positional args should work, code=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "playlist.json")); err != nil {
		t.Fatalf("playlist not written: %v", err)
	}
}
