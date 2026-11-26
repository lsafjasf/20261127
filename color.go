package tearcheck

import "image"

// ChromaEffectAt estimates stylized color splitting/saturation near a seam.
// It is intentionally only one content signal; color alone cannot prove intent.
func ChromaEffectAt(img image.Image, seamY int) float64 {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 8 || height < 8 || seamY <= 1 || seamY >= height-1 {
		return 0
	}

	y0 := maxInt(bounds.Min.Y, seamY-3)
	y1 := minInt(bounds.Max.Y, seamY+4)
	var colorEdges, strongColor int
	for y := y0 + 1; y < y1; y++ {
		for x := bounds.Min.X + 1; x < bounds.Max.X; x += 2 {
			r1, g1, b1, _ := img.At(x, y).RGBA()
			r0, g0, b0, _ := img.At(x, y-1).RGBA()
			r1i, g1i, b1i := int(r1>>8), int(g1>>8), int(b1>>8)
			r0i, g0i, b0i := int(r0>>8), int(g0>>8), int(b0>>8)
			distance := absInt(r1i-r0i) + absInt(g1i-g0i) + absInt(b1i-b0i)
			if distance < 70 {
				continue
			}
			colorEdges++
			maxChroma := maxInt(r1i, maxInt(g1i, b1i)) - minInt3(r1i, g1i, b1i)
			if maxChroma >= 70 || distance >= 180 {
				strongColor++
			}
		}
	}
	if colorEdges == 0 {
		return 0
	}
	fraction := float64(strongColor) / float64(colorEdges)
	return Clamp01((fraction - 0.25) / 0.55)
}

func minInt3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
