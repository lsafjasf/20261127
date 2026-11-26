package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesIncompleteManifestAndReturnsGateFailure(t *testing.T) {
	dir := t.TempDir()
	hdrPath := writeTestFile(t, dir, "hdr.mov", "hdr")
	sdrPath := writeTestFile(t, dir, "sdr.mov", "sdr")
	configPath := filepath.Join(dir, "delivery.json")
	config := `{
	  "project": {"id": "cli-gate"},
	  "hdr": {"path": "hdr.mov"},
	  "sdr": {"path": "sdr.mov"}
	}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = hdrPath
	_ = sdrPath
	outputPath := filepath.Join(dir, "out", "manifest.json")
	var stdout, stderr bytes.Buffer
	exitCode := run(context.Background(), []string{
		"-config", configPath,
		"-out", outputPath,
		"-ffprobe", "/bin/false",
		"-ffmpeg", "/bin/false",
	}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%s", exitCode, stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if manifest["status"] != "incomplete" || manifest["complete"] != false {
		t.Fatalf("unexpected status/complete: %v / %v", manifest["status"], manifest["complete"])
	}
	if !strings.Contains(stderr.String(), "status=incomplete") {
		t.Fatalf("stderr missing status summary: %s", stderr.String())
	}
}

func TestRunAllowIncomplete(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "hdr.mov", "hdr")
	writeTestFile(t, dir, "sdr.mov", "sdr")
	configPath := filepath.Join(dir, "delivery.json")
	config := `{"hdr":{"path":"hdr.mov"},"sdr":{"path":"sdr.mov"}}`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "manifest.md")
	var stdout, stderr bytes.Buffer
	exitCode := run(context.Background(), []string{
		"-config", configPath,
		"-out", outputPath,
		"-format", "markdown",
		"-allow-incomplete",
		"-ffprobe", "/bin/false",
		"-ffmpeg", "/bin/false",
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", exitCode, stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "不完整") || !strings.Contains(content, "逐项检查") {
		t.Fatalf("unexpected markdown output:\n%s", content)
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-config", "missing.json", "-format", "xml"}, &stdout, &stderr); code != 2 {
		t.Fatalf("bad format exit code = %d, want 2", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), nil, &stdout, &stderr); code != 2 {
		t.Fatalf("missing config exit code = %d, want 2", code)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
