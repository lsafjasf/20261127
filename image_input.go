package iccproof

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// SamplesFromPhoto decodes a PNG/JPEG and returns grid samples. A nil grid
// returns a bounded diagnostic grid; set Stride=1 and MaxSamples=-1 for every
// pixel.
func SamplesFromPhoto(path string, grid *SampleGrid) ([]Sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open photo %q: %w", path, err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode photo %q: %w", path, err)
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	stride := 1
	maxSamples := 256
	if grid != nil {
		stride = grid.Stride
		maxSamples = grid.MaxSamples
	}
	if stride <= 0 {
		stride = 1
	}
	if maxSamples == 0 {
		maxSamples = 256
	}
	if maxSamples > 0 {
		for stride < max(width, height) && ((width+stride-1)/stride)*((height+stride-1)/stride) > maxSamples {
			stride++
		}
	}

	var samples []Sample
	index := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stride {
		for x := bounds.Min.X; x < bounds.Max.X; x += stride {
			r, g, b, _ := img.At(x, y).RGBA()
			xx, yy := x, y
			samples = append(samples, Sample{
				Index: index,
				ID:    fmt.Sprintf("x%d_y%d", x, y),
				X:     &xx, Y: &yy,
				RGB: []float64{float64(r) / 65535, float64(g) / 65535, float64(b) / 65535},
			})
			index++
		}
	}
	return samples, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
