package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleManifest = `{
  "version": "1",
  "media_id": "ep01",
  "tracks": [
    {
      "language": "zh",
      "label": "中文",
      "default": true,
      "cues": [
        {"id": "c1", "start": "00:00:01,000", "end": "00:00:03,500", "text": "你好，世界"},
        {"start": "00:00:04.000", "end": "00:00:06.000", "text": "第二行\n续行"}
      ]
    },
    {
      "language": "en",
      "cues": [
        {"start": "00:00:01,000", "end": "00:00:03,500", "text": "Hello, world"}
      ]
    }
  ]
}`

func writeManifest(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildAndVerifyHappyPath(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeManifest(t, dir)
	out := filepath.Join(dir, "dist")

	if err := run([]string{"build", "-manifest", manifestPath, "-out", out}); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	for _, name := range []string{"ep01.zh.srt", "ep01.zh.vtt", "ep01.en.srt", "ep01.en.vtt", "ep01.esub"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing artifact %s: %v", name, err)
		}
	}
	if err := run([]string{"verify", "-manifest", manifestPath, "-out", out}); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}

func TestBuildBlocksOnTamperedSidecar(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeManifest(t, dir)
	out := filepath.Join(dir, "dist")
	if err := run([]string{"build", "-manifest", manifestPath, "-out", out}); err != nil {
		t.Fatal(err)
	}

	srtPath := filepath.Join(out, "ep01.zh.srt")
	original, err := os.ReadFile(srtPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(original), "你好，世界", "被篡改的文本", 1)
	if err := os.WriteFile(srtPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	err = run([]string{"verify", "-manifest", manifestPath, "-out", out})
	if err == nil {
		t.Fatal("expected verification to block release on tampered sidecar")
	}
	if !strings.Contains(err.Error(), "release") &&
		!strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("error should describe blocked release: %v", err)
	}
	if !strings.Contains(err.Error(), "field=text") {
		t.Fatalf("error should pinpoint text field: %v", err)
	}
}

func TestBuildBlocksOnTimingDrift(t *testing.T) {
	dir := t.TempDir()
	manifestPath := writeManifest(t, dir)
	out := filepath.Join(dir, "dist")
	if err := run([]string{"build", "-manifest", manifestPath, "-out", out}); err != nil {
		t.Fatal(err)
	}

	vttPath := filepath.Join(out, "ep01.en.vtt")
	original, err := os.ReadFile(vttPath)
	if err != nil {
		t.Fatal(err)
	}
	drifted := strings.Replace(string(original), "00:00:03.500", "00:00:09.999", 1)
	if err := os.WriteFile(vttPath, []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}

	err = run([]string{"verify", "-manifest", manifestPath, "-out", out})
	if err == nil || !strings.Contains(err.Error(), "field=end") {
		t.Fatalf("expected end-time mismatch, got: %v", err)
	}
}

func TestInvalidManifestRejected(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"media_id":"x","tracks":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"build", "-manifest", bad, "-out", filepath.Join(dir, "out")}); err == nil {
		t.Fatal("expected invalid manifest error")
	}
}
