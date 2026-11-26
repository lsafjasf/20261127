package pipeline

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"colorfix/internal/analyze"
	"colorfix/internal/color"
	"colorfix/internal/fixture"
	"colorfix/internal/video"
)

func TestEndToEndMislabeled601(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not available")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "mislabelled.mp4")
	raw := bytes.Repeat(fixture.YUV420Frame(720, 576), 15)
	if err := fixture.WriteMislabeled601(ffmpeg, raw, input, 720, 576, 15, 25); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	opts := Options{
		FFmpeg: ffmpeg, FFprobe: ffprobe, OutDir: outDir,
		OpenSeconds: 1, OpenFPS: 4, FallbackCount: 8,
		Decision: analyze.DefaultDecisionOptions(),
	}
	report, err := Process(input, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Decision.Status != analyze.StatusRelabeled || report.Decision.Selected != color.BT601 {
		t.Fatalf("decision = %+v", report.Decision)
	}
	s601 := report.Bars.Score(color.BT601)
	s709 := report.Bars.Score(color.BT709)
	if s601.Residual > 0.015 || s709.Residual < 0.03 {
		t.Fatalf("unexpected residuals: 601 %.4f, 709 %.4f", s601.Residual, s709.Residual)
	}
	if report.Fixed == nil {
		t.Fatal("unique BT.601 decision did not produce corrected media")
	}
	fixedProbe, err := video.ProbeFile(ffprobe, report.Fixed.Path)
	if err != nil {
		t.Fatal(err)
	}
	if fixedProbe.Matrix != color.BT601 {
		t.Fatalf("fixed matrix = %q, want bt601", fixedProbe.MatrixName)
	}
	if report.BarsFrame == nil || report.Content == nil {
		t.Fatal("comparison frames were not written")
	}
	assertPNGSize(t, filepath.Join(report.OutputDir, report.BarsFrame.Frames.Compare), 1450, 602)
	assertPNGSize(t, filepath.Join(report.OutputDir, report.Content.Frames.Compare), 1450, 602)
	assertLabelSidecar(t, filepath.Join(report.OutputDir, "label.json"), color.BT601)
}

func TestEndToEndAmbiguousKeepsCandidates(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not available")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "ambiguous.mp4")
	raw := bytes.Repeat(fixture.YUV420FrameBlend(720, 576, 0.5), 12)
	if err := fixture.WriteMislabeled601(ffmpeg, raw, input, 720, 576, 12, 25); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	opts := Options{
		FFmpeg: ffmpeg, FFprobe: ffprobe, OutDir: outDir,
		OpenSeconds: 1, OpenFPS: 4, FallbackCount: 8,
		Decision: analyze.DefaultDecisionOptions(),
	}
	report, err := Process(input, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Decision.Status != analyze.StatusAmbiguous {
		t.Fatalf("status = %q, want ambiguous", report.Decision.Status)
	}
	if report.Decision.Selected != "" || len(report.Decision.Candidates) != 2 {
		t.Fatalf("ambiguous result was forced: %+v", report.Decision)
	}
	if report.Fixed != nil {
		t.Fatal("ambiguous evidence must not write corrected media")
	}
	if report.Labels.Recommended != nil || len(report.Labels.Candidates) != 2 {
		t.Fatalf("labels did not preserve candidates: %+v", report.Labels)
	}
}

func assertPNGSize(t *testing.T, path string, width, height int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != width || img.Height != height {
		t.Fatalf("%s: got %dx%d, want %dx%d", path, img.Width, img.Height, width, height)
	}
}

func assertLabelSidecar(t *testing.T, path string, want color.Matrix) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Status   string `json:"status"`
		Selected string `json:"selected_matrix"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Status != analyze.StatusRelabeled || doc.Selected != string(want) {
		t.Fatalf("label sidecar = %+v", doc)
	}
}
