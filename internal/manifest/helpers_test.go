package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseMediaHelpers(t *testing.T) {
	if value, ok := SecondsToMS("1.2345"); !ok || value != 1235 {
		t.Fatalf("SecondsToMS() = %d, %v", value, ok)
	}
	if _, ok := SecondsToMS("N/A"); ok {
		t.Fatal("expected invalid duration")
	}
	if value, ok := ParseRational("24000/1001"); !ok || value < 23.97 || value > 23.98 {
		t.Fatalf("ParseRational() = %f, %v", value, ok)
	}
	for format, expected := range map[string]int{
		"yuv420p":     8,
		"yuv420p10le": 10,
		"p010le":      10,
		"yuv444p12le": 12,
	} {
		if depth := PixelBitDepth(ProbeStream{PixelFormat: format}); depth != expected {
			t.Errorf("PixelBitDepth(%q) = %d, want %d", format, depth, expected)
		}
	}
}

func TestJSONStringAcceptsNumberAndText(t *testing.T) {
	var chapter ProbeChapter
	if err := json.Unmarshal([]byte(`{"id": 7, "start_time": "0.000000"}`), &chapter); err != nil {
		t.Fatal(err)
	}
	if chapter.ID != "7" {
		t.Fatalf("numeric chapter ID = %q", chapter.ID)
	}
	if err := json.Unmarshal([]byte(`{"id": "opening", "start_time": "0.000000"}`), &chapter); err != nil {
		t.Fatal(err)
	}
	if chapter.ID != "opening" {
		t.Fatalf("text chapter ID = %q", chapter.ID)
	}
}

func TestLoadCutPointsSortsAndValidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cuts.json")
	content := `{"cuts":[
	  {"id":"C2","time_ms":1000,"label":"Scene 2"},
	  {"id":"C1","time_ms":0,"label":"Opening"}
	]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	points, err := LoadCutPoints(CutsConfig{Path: path})
	if err != nil {
		t.Fatalf("LoadCutPoints() error = %v", err)
	}
	if len(points) != 2 || points[0].ID != "C1" || points[1].ID != "C2" {
		t.Fatalf("cuts not sorted: %+v", points)
	}

	duplicate := filepath.Join(dir, "duplicate.json")
	if err := os.WriteFile(duplicate, []byte(`[
	  {"id":"C1","time_ms":0},
	  {"id":"C2","time_ms":0}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCutPoints(CutsConfig{Path: duplicate}); err == nil {
		t.Fatal("expected duplicate cut time error")
	}
}

func TestMatchCutsToleranceAndUnexpected(t *testing.T) {
	expected := []CutPoint{
		{ID: "C1", TimeMS: 1000},
		{ID: "C2", TimeMS: 2000},
	}
	actual := []CutMarker{
		{ID: "a", TimeMS: 1001},
		{ID: "b", TimeMS: 2500},
	}
	missing, unexpected := MatchCuts(expected, actual, 2)
	if len(missing) != 1 || missing[0].ID != "C2" {
		t.Fatalf("missing = %+v", missing)
	}
	if len(unexpected) != 1 || unexpected[0].ID != "b" {
		t.Fatalf("unexpected = %+v", unexpected)
	}
}

func TestCutSourceNormalization(t *testing.T) {
	tests := []struct {
		config CutsConfig
		want   string
	}{
		{config: CutsConfig{}, want: "paired"},
		{config: CutsConfig{Points: []CutPoint{{ID: "C1"}}}, want: "list"},
		{config: CutsConfig{Path: "cuts.json"}, want: "list"},
		{config: CutsConfig{Source: "paired"}, want: "paired"},
	}
	for _, test := range tests {
		if got := NormalizeCutSource(test.config); got != test.want {
			t.Errorf("NormalizeCutSource(%+v) = %q, want %q", test.config, got, test.want)
		}
	}
}
