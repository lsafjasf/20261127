package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDemoThenCheckGate(t *testing.T) {
	tmp := t.TempDir()
	args := []string{
		"-dir", filepath.Join(tmp, "ladder"),
		"-json", filepath.Join(tmp, "report.json"),
		"-md", filepath.Join(tmp, "report.md"),
		"-constraint", filepath.Join(tmp, "constraint.json"),
	}
	if err := runDemo(args); err != nil {
		t.Fatalf("runDemo: %v", err)
	}
	for _, p := range []string{"report.json", "report.md", "constraint.json", "ladder/rung-360p.mp4"} {
		if _, err := os.Stat(filepath.Join(tmp, p)); err != nil {
			t.Fatalf("expected output %s: %v", p, err)
		}
	}

	// The generated incident ladder must fail the switch-safe gate.
	err := runAnalyze([]string{"-check", "-fps", "30/1", "-gop-frames", "60", filepath.Join(tmp, "ladder")})
	var ce *checkError
	if !errors.As(err, &ce) {
		t.Fatalf("expected checkError, got %v", err)
	}
}

func TestAlignedManifestPassesCheck(t *testing.T) {
	if err := runAnalyze([]string{"-check", "-in", filepath.Join("..", "..", "examples", "ladder.aligned.json")}); err != nil {
		t.Fatalf("aligned manifest should pass -check: %v", err)
	}
}
