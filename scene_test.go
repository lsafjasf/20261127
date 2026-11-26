package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestSampleSceneHasReferenceAndClipping(t *testing.T) {
	encoded, info, err := buildSampleScene()
	if err != nil {
		t.Fatalf("build sample scene: %v", err)
	}
	if info.Width != sampleWidth || info.Height != sampleHeight {
		t.Fatalf("unexpected scene size: %dx%d", info.Width, info.Height)
	}

	source, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode sample scene: %v", err)
	}
	refPixelX := int(info.Reference.X * float64(info.Width))
	refPixelY := int(info.Reference.Y * float64(info.Height))
	r, g, b, _ := source.At(refPixelX, refPixelY).RGBA()
	rr, gg, bb := int(r>>8), int(g>>8), int(b>>8)
	if maxInt(rr, gg, bb)-minInt(rr, gg, bb) > 4 {
		t.Fatalf("reference patch is not neutral: rgb(%d,%d,%d)", rr, gg, bb)
	}
	if rr < 108 || rr > 128 {
		t.Fatalf("reference patch has unexpected 18%% tone: rgb(%d,%d,%d)", rr, gg, bb)
	}

	clipped := countClipped(source, image.Rect(900, 60, 1400, 500))
	if clipped < 5000 {
		t.Fatalf("expected a broad clipped window region, found %d clipped pixels", clipped)
	}
}

func countClipped(img image.Image, bounds image.Rectangle) int {
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 >= 250 && g>>8 >= 250 && b>>8 >= 250 {
				count++
			}
		}
	}
	return count
}

func maxInt(a, b, c int) int {
	return max(a, max(b, c))
}

func minInt(a, b, c int) int {
	return min(a, min(b, c))
}
