package check_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"proalpha/internal/check"
	"proalpha/internal/fixture"
)

func TestFixtureDetectsAlphaAndColorErrors(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	root := t.TempDir()
	paths, err := fixture.Generate(filepath.Join(root, "fixtures"), 2, 64, 36)
	if err != nil {
		t.Fatal(err)
	}
	reportDir := filepath.Join(root, "report")
	report, err := check.Run(check.Options{
		SourcePath:   paths.SourceProRes,
		VariantPaths: []string{paths.DroppedProxy, paths.PremulProxy},
		OutputDir:    reportDir,
		MaxFrames:    2,
	})
	if err != nil {
		t.Fatal(err)
	}

	codes := map[string]bool{}
	for _, param := range report.WrongParameters {
		codes[param.Code] = true
	}
	for _, code := range []string{
		"alpha_flag_missing_after_transcode",
		"color_descriptor_mismatch",
		"pixel_alpha_channel_dropped",
		"pixel_premultiplied_rgb_misread_as_straight",
	} {
		if !codes[code] {
			t.Fatalf("expected finding %q; codes=%v", code, codes)
		}
	}

	matches, err := filepath.Glob(filepath.Join(reportDir, "*_repaired.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) < 4 {
		t.Fatalf("repaired PNG count = %d, want at least 4", len(matches))
	}
}
