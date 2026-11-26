package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "observermap")
	cmd := exec.Command("go", "build", "-o", exe, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
	return exe
}

func runBin(t *testing.T, exe string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(exe, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func TestCLIEndToEnd(t *testing.T) {
	exe := buildBinary(t)
	td := filepath.Join("..", "..", "testdata")

	out, code := runBin(t, exe)
	if code != 0 || !strings.Contains(out, "EXPERIMENTAL EMPIRICAL") {
		t.Fatalf("bare demo code=%d, output missing disclaimer", code)
	}

	out, code = runBin(t, exe, "demo", "--json")
	if code != 0 {
		t.Fatalf("demo --json code %d", code)
	}
	var demo map[string]any
	if err := json.Unmarshal([]byte(out), &demo); err != nil {
		t.Fatalf("demo json: %v", err)
	}
	if demo["disclaimer"] == nil {
		t.Error("json missing disclaimer")
	}

	if _, code = runBin(t, exe, "inspect", filepath.Join(td, "illuminant-a-white.icc"), "--strict"); code != 3 {
		t.Errorf("unsupported white --strict code=%d want 3", code)
	}
	if _, code = runBin(t, exe, "inspect", filepath.Join(td, "monitor-d65.icc"), "--strict"); code != 0 {
		t.Errorf("supported d65 --strict code=%d want 0", code)
	}
	if _, code = runBin(t, exe, "inspect", filepath.Join(t.TempDir(), "missing.icc")); code != 1 {
		t.Errorf("missing profile code=%d want 1", code)
	}

	out, code = runBin(t, exe, "map", "--hex", "#0000FF", "--json")
	if code != 0 {
		t.Fatalf("map json code=%d out=%s", code, out)
	}
	var mr map[string]any
	if err := json.Unmarshal([]byte(out), &mr); err != nil {
		t.Fatalf("map json: %v", err)
	}
	if mr["standard_conversion"] != false || mr["empirical"] != true {
		t.Error("mapping must be labelled empirical and non-standard")
	}

	// Flags after a positional path must still parse.
	if _, code = runBin(t, exe, "inspect", filepath.Join(td, "monitor-d50.icc"), "--json"); code != 0 {
		t.Errorf("flags-after-positional code=%d", code)
	}

	if _, code = runBin(t, exe, "map"); code != 2 {
		t.Errorf("map without input code=%d want 2", code)
	}
}
