package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lsafjasf/20261127/cadence"
)

func TestExtractAndAnalyzeMixedTelecine(t *testing.T) {
	if testing.Short() {
		t.Skip("ffmpeg integration test")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	path := filepath.Join(t.TempDir(), "mixed-tff.mp4")
	generateMixed(t, path, "top")

	decoded, err := Extract(context.Background(), path, Options{FieldWidth: 32, FieldHeight: 18})
	if err != nil {
		t.Fatal(err)
	}
	report, err := cadence.Analyze(decoded, cadence.Options{WindowFrames: 16, MinSegmentFrames: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Segments) == 0 || report.Segments[0].StartFrame != 0 ||
		report.Segments[len(report.Segments)-1].EndFrame != len(decoded.Frames) {
		t.Fatalf("segments do not cover the decoded video: %+v", report.Segments)
	}
	assertCadenceSequence(t, report, cadence.Cadence24p32, cadence.Cadence30p22)
	for _, segment := range report.Segments {
		if segment.Cadence == cadence.Cadence24p32 && segment.FieldOrder != cadence.OrderTFF {
			t.Fatalf("TFF telecine field order = %s", segment.FieldOrder)
		}
	}
}

func TestExtractAndAnalyzeBFFTelecine(t *testing.T) {
	if testing.Short() {
		t.Skip("ffmpeg integration test")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "film-bff.mp4")
	generateFilm(t, path, "bottom")

	decoded, err := Extract(context.Background(), path, Options{FieldWidth: 32, FieldHeight: 18})
	if err != nil {
		t.Fatal(err)
	}
	report, err := cadence.Analyze(decoded, cadence.Options{WindowFrames: 16, MinSegmentFrames: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Segments) != 1 {
		t.Fatalf("got %d segments for one cadence: %+v", len(report.Segments), report.Segments)
	}
	segment := report.Segments[0]
	if segment.Cadence != cadence.Cadence24p32 || segment.FieldOrder != cadence.OrderBFF {
		t.Fatalf("got %s/%s, want 24p_3:2/bff", segment.Cadence, segment.FieldOrder)
	}
}

func generateMixed(t *testing.T, path, fieldOrder string) {
	t.Helper()
	dir := t.TempDir()
	film := filepath.Join(dir, "film.mp4")
	progressive := filepath.Join(dir, "30p.mp4")
	list := filepath.Join(dir, "concat.txt")
	filmArgs := []string{
		"-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=2",
		"-vf", "telecine=first_field=" + fieldOrder + ":pattern=23",
		"-an",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		film,
	}
	if out, err := exec.Command("ffmpeg", filmArgs...).CombinedOutput(); err != nil {
		t.Fatalf("generate mixed video: %v\n%s", err, out)
	}
	p30Args := []string{
		"-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=30:duration=2",
		"-an", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		progressive,
	}
	if out, err := exec.Command("ffmpeg", p30Args...).CombinedOutput(); err != nil {
		t.Fatalf("generate 30p video: %v\n%s", err, out)
	}
	if err := os.WriteFile(list, []byte("file '"+film+"'\nfile '"+progressive+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	concatArgs := []string{
		"-v", "error", "-y", "-f", "concat", "-safe", "0", "-i", list,
		"-c", "copy", path,
	}
	if out, err := exec.Command("ffmpeg", concatArgs...).CombinedOutput(); err != nil {
		t.Fatalf("concat mixed video: %v\n%s", err, out)
	}
}

func generateFilm(t *testing.T, path, fieldOrder string) {
	t.Helper()
	args := []string{
		"-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=2",
		"-vf", "telecine=first_field=" + fieldOrder + ":pattern=23",
		"-an", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		path,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("generate film video: %v\n%s", err, out)
	}
}

func assertCadenceSequence(t *testing.T, report cadence.Report, expected ...cadence.Cadence) {
	t.Helper()
	var got []cadence.Cadence
	for _, segment := range report.Segments {
		if segment.Cadence == cadence.CadenceUnknown {
			continue
		}
		if len(got) == 0 || got[len(got)-1] != segment.Cadence {
			got = append(got, segment.Cadence)
		}
	}
	if len(got) != len(expected) {
		t.Fatalf("cadence sequence = %v, want %v; report=%+v", got, expected, report.Segments)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("cadence sequence = %v, want %v", got, expected)
		}
	}
}
