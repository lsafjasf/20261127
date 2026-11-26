package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInterspersed(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	count := flags.Int("count", 0, "")
	name := flags.String("name", "", "")

	positionals, err := parseInterspersed(flags, []string{"video.mp4", "--count", "7", "--name=cam", "extra"})
	if err != nil {
		t.Fatal(err)
	}
	if len(positionals) != 2 || positionals[0] != "video.mp4" || positionals[1] != "extra" {
		t.Fatalf("positionals=%v", positionals)
	}
	if *count != 7 || *name != "cam" {
		t.Fatalf("count=%d name=%q", *count, *name)
	}
}

func TestRunSignalsExample(t *testing.T) {
	output := filepath.Join(t.TempDir(), "report.json")
	err := run([]string{
		"signals",
		"--input", filepath.Join("..", "..", "examples", "request.json"),
		"--output", output,
		"--format", "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"overall": "hardware_sync"`) {
		t.Fatalf("report missing overall verdict: %s", data)
	}
}
