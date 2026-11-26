package rangecheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessRawGrayFileAndWriters(t *testing.T) {
	dir := t.TempDir()
	rawPath := filepath.Join(dir, "frames.gray")
	const w, h = 48, 32
	frame := make([]byte, w*h)
	for i := range frame {
		frame[i] = 200
	}
	for x := 4; x < 20; x++ {
		frame[(h-8)*w+x] = 0
	}
	if err := os.WriteFile(rawPath, append(append([]byte(nil), frame...), frame...), 0o644); err != nil {
		t.Fatal(err)
	}

	settings := DefaultSettings(w, h)
	settings.PixelFormat = FormatGray8
	settings.FPS = Rational{Num: 12, Den: 1}
	analyzer := NewAnalyzer(rawPath, settings, Metadata{})
	report, err := ProcessPath(rawPath, analyzer, ProcessOptions{Kind: InputRaw, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 2 {
		t.Fatalf("frames = %d, want 2", len(report.Frames))
	}
	if len(report.Segments) != 1 || report.Segments[0].FrameCount != 2 {
		t.Fatalf("segments = %+v", report.Segments)
	}

	jsonPath := filepath.Join(dir, "report.json")
	csvPath := filepath.Join(dir, "frames.csv")
	mdPath := filepath.Join(dir, "report.md")
	if err := WriteJSON(report, jsonPath); err != nil {
		t.Fatal(err)
	}
	if err := WriteFrameCSV(report, csvPath); err != nil {
		t.Fatal(err)
	}
	if err := WriteMarkdown(report, mdPath); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{jsonPath, csvPath, mdPath} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("output %s missing or empty: %v", path, err)
		}
	}
}
