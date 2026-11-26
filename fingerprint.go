package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// Fingerprint computes a 64-bit difference hash (dHash) of a frame:
// the image is downsampled to 9x8 luma samples and each bit records
// whether a sample is brighter than its right neighbour. Robust to
// recompression and res scaling, which is what a render round-trip does.
func Fingerprint(img image.Image) uint64 {
	const w, h = 9, 8
	b := img.Bounds()
	var bits uint64
	prev := lumaAt(img, b, 0, 0, w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cur := lumaAt(img, b, x, y, w, h)
			if x > 0 {
				bits <<= 1
				if prev > cur {
					bits |= 1
				}
			}
			prev = cur
		}
	}
	return bits
}

func lumaAt(img image.Image, b image.Rectangle, x, y, w, h int) uint32 {
	px := b.Min.X + (x*b.Dx()+b.Dx()/2)/w
	py := b.Min.Y + (y*b.Dy()+b.Dy()/2)/h
	r, g, bl, _ := img.At(px, py).RGBA()
	// ITU-R BT.601 luma, 16-bit domain.
	return (299*r + 587*g + 114*bl) / 1000
}

// FingerprintFile decodes an image frame (PNG/JPEG/GIF) and hashes it.
func FingerprintFile(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("decode %s: %w", path, err)
	}
	return Fingerprint(img), nil
}

// Hamming distance between two fingerprints, for near-duplicate checks.
func Hamming(a, b uint64) int {
	d := 0
	for v := a ^ b; v != 0; v &= v - 1 {
		d++
	}
	return d
}
