package main

import (
	"encoding/csv"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"flatfield"
)

func writePNG(t *testing.T, path string, w, h int, pixel func(u, v float64) color.NRGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := (float64(y) + .5) / float64(h)
		for x := 0; x < w; x++ {
			u := (float64(x) + .5) / float64(w)
			img.SetNRGBA(x, y, pixel(u, v))
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestCLIExportsTableAndCorrectsImage(t *testing.T) {
	dir := t.TempDir()
	flatPath := filepath.Join(dir, "flat.png")
	scenePath := filepath.Join(dir, "scene.png")
	outPath := filepath.Join(dir, "corrected.png")
	tablePath := filepath.Join(dir, "table.json")
	csvPath := filepath.Join(dir, "table.csv")

	aspect := 96.0 / 72.0
	rScale := math.Hypot(.5*aspect, .5)
	writePNG(t, flatPath, 96, 72, func(u, v float64) color.NRGBA {
		dx := (u - .5) * aspect
		dy := v - .5
		r2 := (math.Hypot(dx, dy) / rScale)
		r2 *= r2
		resp := math.Exp(-.28*r2 - .10*r2*r2 + .10*dx - .12*dy + .14*(2*u-1)*(2*v-1))
		level := uint8(math.Round(210 * math.Min(1, resp)))
		return color.NRGBA{R: level, G: level, B: level, A: 255}
	})
	writePNG(t, scenePath, 64, 48, func(u, v float64) color.NRGBA {
		r := uint8(120 + 80*u)
		g := uint8(90 + 70*v)
		b := uint8(160 - 50*u)
		return color.NRGBA{R: r, G: g, B: b, A: 255}
	})

	err := run([]string{
		"-flat", flatPath,
		"-in", scenePath,
		"-out", outPath,
		"-table-out", tablePath,
		"-csv-out", csvPath,
		"-grid-w", "20",
		"-quiet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flatfield.LoadTable(tablePath); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(csvPath); err != nil || info.Size() == 0 {
		t.Fatalf("csv not exported: %v size=%d", err, func() int64 {
			i, e := os.Stat(csvPath)
			if e != nil {
				return 0
			}
			return i.Size()
		}())
	}
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 48 {
		t.Fatalf("output bounds = %v", img.Bounds())
	}
}

func TestCLIRequiresFlat(t *testing.T) {
	if err := run([]string{"-quiet"}); err == nil {
		t.Fatal("expected error without -flat")
	}
}

func TestCLIAppliesExportedTableAndResamplesCSV(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	flatPath := filepath.Join(dir, "flat.png")
	scenePath := filepath.Join(dir, "scene-large.png")
	tablePath := filepath.Join(dir, "table.json")
	resizedCSV := filepath.Join(dir, "gain-48x36.csv")
	outPath := filepath.Join(dir, "corrected-large.png")
	gradientPath := filepath.Join(dir, "scene-gradient.json")

	aspect := 96.0 / 72.0
	rScale := math.Hypot(.5*aspect, .5)
	writePNG(t, flatPath, 96, 72, func(u, v float64) color.NRGBA {
		dx := (u - .5) * aspect
		dy := v - .5
		r2 := math.Hypot(dx, dy) / rScale
		r2 *= r2
		resp := math.Exp(-.24*r2 - .08*r2*r2 + .08*dx - .10*dy + .12*(2*u-1)*(2*v-1))
		level := uint8(math.Round(205 * math.Min(1, resp)))
		return color.NRGBA{R: level, G: level, B: level, A: 255}
	})
	writePNG(t, scenePath, 128, 96, func(u, v float64) color.NRGBA {
		return color.NRGBA{R: 180, G: 120, B: 90, A: 255}
	})

	if err := run([]string{
		"-flat", flatPath,
		"-table-out", tablePath,
		"-grid-w", "16",
		"-quiet",
	}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{
		"-apply-table", tablePath,
		"-in", scenePath,
		"-out", outPath,
		"-csv-out", resizedCSV,
		"-csv-width", "48",
		"-csv-height", "36",
		"-scene-gradient-out", gradientPath,
		"-quiet",
	}); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(resizedCSV)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 36 || len(records[0]) != 48 {
		t.Fatalf("resampled CSV = %dx%d records, want 48x36", len(records), len(records[0]))
	}
	info, err := os.Stat(outPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("corrected image missing: %v", err)
	}
	data, err := os.ReadFile(gradientPath)
	if err != nil {
		t.Fatal(err)
	}
	var gradient flatfield.GradientStats
	if err := json.Unmarshal(data, &gradient); err != nil {
		t.Fatal(err)
	}
	if gradient.Interpretation == "" {
		t.Fatal("scene gradient diagnostic missing interpretation")
	}
	if _, err := os.Stat("flatfield-table.json"); !os.IsNotExist(err) {
		t.Fatalf("apply-table mode must not create default table output, stat err=%v", err)
	}
}
