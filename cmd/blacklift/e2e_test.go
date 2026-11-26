package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"blacklift/internal/frameio"
)

func TestEndToEndScenes(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}

	// Render frames by invoking the shared scene renderer indirectly is not
	// exported; instead exercise the full CLI through run() with generated
	// frames is covered via genscenes on disk in TestMain-free integration.
	// Here we validate run() against fixtures produced with genscenes when
	// present, otherwise skip.
	if _, err := os.Stat("../../testdata/fog_cropped"); err != nil {
		t.Skip("synthetic fixtures missing; run: go run ./cmd/genscenes")
	}

	cases := []struct {
		scene       string
		wantLift    bool
		wantApplied bool
	}{
		{scene: "fog_cropped", wantLift: false, wantApplied: true},
		{scene: "true_night", wantLift: false, wantApplied: false},
		{scene: "intentional_lift", wantLift: true, wantApplied: false},
	}
	for _, tc := range cases {
		t.Run(tc.scene, func(t *testing.T) {
			outDir := filepath.Join(tmp, tc.scene)
			cfg := config{
				inDir:    filepath.Join("..", "..", "testdata", tc.scene),
				outDir:   outDir,
				csvPath:  filepath.Join(tmp, tc.scene+".csv"),
				jsonPath: filepath.Join(tmp, tc.scene+".json"),
				cols:     16, rows: 9, minTile: 24,
				maxComp: 16, noiseK: 1.5, styleScale: 0,
				styleThresh: 0.62, smoothAlpha: 0.65, voteWindow: 5,
				forceStyle: "auto",
			}
			if err := run(cfg); err != nil {
				t.Fatal(err)
			}
			paths, _ := frameio.ListFrames(cfg.inDir)
			before, _ := frameio.Load(paths[0])
			after, err := frameio.Load(filepath.Join(outDir, filepath.Base(paths[0])))
			if err != nil {
				t.Fatal(err)
			}
			changed := 0
			for i := range before.Y {
				if math.Abs(before.Y[i]-after.Y[i]) > 1e-9 {
					changed++
				}
			}
			if tc.wantApplied && changed == 0 {
				t.Fatalf("%s: expected pixel corrections, got none", tc.scene)
			}
			if !tc.wantApplied && changed != 0 {
				t.Fatalf("%s: expected untouched pixels, %d changed", tc.scene, changed)
			}
			// Corrected fog must get darker in shadows, never brighter.
			if tc.wantApplied {
				meanBefore, meanAfter := 0.0, 0.0
				dark := 0
				for i := range before.Y {
					if before.Y[i] < 0.1 {
						dark++
						meanBefore += before.Y[i]
						meanAfter += after.Y[i]
					}
				}
				if dark == 0 || meanAfter >= meanBefore {
					t.Fatalf("%s: shadows not deepened", tc.scene)
				}
			}
		})
	}
}
