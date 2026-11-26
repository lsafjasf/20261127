package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (int, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	code := RunCLI(args, nil, &out, &errb)
	return code, &out, &errb
}

func TestCLIGenThenAnalyzeJSON(t *testing.T) {
	dir := t.TempDir() + "/sc"
	code, out, errb := runCLI(t, "gen", "--out", dir)
	if code != 0 {
		t.Fatalf("gen code=%d stderr=%s", code, errb.String())
	}

	// Flags after the positional directory must still be parsed.
	code, out, errb = runCLI(t, "analyze", dir, "--json")
	if code != 0 {
		t.Fatalf("analyze code=%d stderr=%s", code, errb.String())
	}
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("json: %v\noutput head: %.200s", err, out.String())
	}
	if rep.First == nil || rep.First.Kind != KindCaptureDrop {
		t.Fatalf("first = %+v", rep.First)
	}

	code, _, _ = runCLI(t, "analyze", "--strict", dir)
	if code != 1 {
		t.Fatalf("strict on anomalies code=%d, want 1", code)
	}
}

func TestCLITextReport(t *testing.T) {
	dir := t.TempDir() + "/sc"
	if code, _, e := runCLI(t, "gen", "--out", dir); code != 0 {
		t.Fatalf("gen: %s", e.String())
	}
	code, out, errb := runCLI(t, "analyze", "--all-windows", dir)
	if code != 0 {
		t.Fatalf("analyze: %s", errb.String())
	}
	txt := out.String()
	for _, want := range []string{"应有/实际/画面推进", "采集丢帧", "编码重复帧", "播放器跳帧", "首个异常", "WIN"} {
		if !strings.Contains(txt, want) {
			t.Errorf("text report missing %q", want)
		}
	}
}

func TestCLICleanStrictZero(t *testing.T) {
	dir := t.TempDir()
	tr := buildHashTrace(120, 30, 3, nil, nil)
	p := dir + "/record.jsonl"
	if err := writeTraceJSONL(p, tr); err != nil {
		t.Fatal(err)
	}
	code, _, errb := runCLI(t, "analyze", "--strict", p)
	if code != 0 {
		t.Fatalf("clean strict code=%d stderr=%s", code, errb.String())
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	code, _, _ := runCLI(t, "frobnicate")
	if code != 2 {
		t.Fatalf("unknown command code=%d want 2", code)
	}
}
