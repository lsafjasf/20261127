package iccproof

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSamplesFromPhotoAndProofSamePixelsAcrossIntents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 255, G: 0, B: 255, A: 255})
	img.SetRGBA(0, 1, color.RGBA{R: 128, G: 128, B: 128, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	samples, err := SamplesFromPhoto(path, &SampleGrid{Stride: 1, MaxSamples: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 4 {
		t.Fatalf("samples=%d want 4", len(samples))
	}
	if samples[0].X == nil || samples[0].Y == nil || len(samples[0].RGB) != 3 {
		t.Fatalf("sample missing coordinates/RGB: %+v", samples[0])
	}

	source, media := testProfiles()
	report, err := Proof(Request{
		PhotoPath: path, Source: source, Media: media,
		Intents: AllProofIntents(), Samples: samples,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 12 {
		t.Fatalf("results=%d want 12", len(report.Results))
	}
}
