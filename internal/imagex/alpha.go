package imagex

import (
	"image"
	"math"
)

const Max16 = 65535

type AlphaStats struct {
	Pixels           int     `json:"pixels"`
	Min              uint16  `json:"min"`
	Max              uint16  `json:"max"`
	Transparent      int     `json:"transparent"`
	Partial          int     `json:"partial"`
	Opaque           int     `json:"opaque"`
	NonOpaque        int     `json:"non_opaque"`
	MeanAlpha        float64 `json:"mean_alpha"`
	RGBExceedsAlpha  int     `json:"rgb_exceeds_alpha"`
	PremulCompatible int     `json:"premul_compatible"`
	StraightEvidence int     `json:"straight_evidence"`
}

type AlphaModel string

const (
	AlphaModelOpaque              AlphaModel = "opaque"
	AlphaModelStraightConfirmed   AlphaModel = "straight_confirmed"
	AlphaModelLikelyPremultiplied AlphaModel = "likely_premultiplied"
	AlphaModelAmbiguous           AlphaModel = "ambiguous"
)

func alphaAt(f *image.NRGBA64, i int) uint16 {
	o := i * 8
	return uint16(f.Pix[o+6])<<8 | uint16(f.Pix[o+7])
}

func channelsAt(f *image.NRGBA64, i int) (uint16, uint16, uint16, uint16) {
	o := i * 8
	r := uint16(f.Pix[o+0])<<8 | uint16(f.Pix[o+1])
	g := uint16(f.Pix[o+2])<<8 | uint16(f.Pix[o+3])
	b := uint16(f.Pix[o+4])<<8 | uint16(f.Pix[o+5])
	a := uint16(f.Pix[o+6])<<8 | uint16(f.Pix[o+7])
	return r, g, b, a
}

func AnalyzeAlpha(f *image.NRGBA64) AlphaStats {
	stats := AlphaStats{Pixels: f.Rect.Dx() * f.Rect.Dy(), Min: Max16, Max: 0}
	if stats.Pixels == 0 {
		stats.Min = 0
		return stats
	}

	tolerance := uint32(math.Round(float64(Max16) * 0.02))
	var sum uint64
	for i := 0; i < stats.Pixels; i++ {
		r, g, b, a := channelsAt(f, i)
		sum += uint64(a)
		if a < stats.Min {
			stats.Min = a
		}
		if a > stats.Max {
			stats.Max = a
		}

		switch {
		case uint32(a) <= tolerance:
			stats.Transparent++
		case uint32(a) >= Max16-tolerance:
			stats.Opaque++
		default:
			stats.Partial++
		}
		if uint32(a) < Max16-tolerance {
			stats.NonOpaque++
		}
		if uint32(a) > tolerance && uint32(a) < Max16-tolerance {
			maxRGB := maxU16(r, g, b)
			if uint32(maxRGB) > uint32(a)+tolerance {
				stats.RGBExceedsAlpha++
				stats.StraightEvidence++
			}
			if uint32(r) <= uint32(a)+tolerance &&
				uint32(g) <= uint32(a)+tolerance &&
				uint32(b) <= uint32(a)+tolerance {
				stats.PremulCompatible++
			}
		}
	}
	stats.MeanAlpha = float64(sum) / float64(stats.Pixels)
	return stats
}

func ClassifySingleAlpha(f *image.NRGBA64) (AlphaModel, AlphaStats) {
	stats := AnalyzeAlpha(f)
	if stats.NonOpaque == 0 {
		return AlphaModelOpaque, stats
	}
	partial := stats.Partial
	if partial == 0 {
		return AlphaModelAmbiguous, stats
	}
	if stats.StraightEvidence*4 >= partial && stats.StraightEvidence >= 3 {
		return AlphaModelStraightConfirmed, stats
	}
	if stats.PremulCompatible*4 >= partial*3 && stats.RGBExceedsAlpha == 0 {
		return AlphaModelLikelyPremultiplied, stats
	}
	return AlphaModelAmbiguous, stats
}

func Unpremultiply(src *image.NRGBA64) *image.NRGBA64 {
	dst := image.NewNRGBA64(src.Rect)
	copy(dst.Pix, src.Pix)
	n := src.Rect.Dx() * src.Rect.Dy()
	for i := 0; i < n; i++ {
		r, g, b, a := channelsAt(src, i)
		o := i * 8
		if a == 0 {
			clear(dst.Pix[o : o+8])
			dst.Pix[o+6], dst.Pix[o+7] = 0xff, 0xff
			continue
		}
		scale := float64(Max16) / float64(a)
		putChannel(dst.Pix[o+0:o+2], clamp16(float64(r)*scale))
		putChannel(dst.Pix[o+2:o+4], clamp16(float64(g)*scale))
		putChannel(dst.Pix[o+4:o+6], clamp16(float64(b)*scale))
		putChannel(dst.Pix[o+6:o+8], a)
	}
	return dst
}

func Premultiply(src *image.NRGBA64) *image.NRGBA64 {
	dst := image.NewNRGBA64(src.Rect)
	n := src.Rect.Dx() * src.Rect.Dy()
	for i := 0; i < n; i++ {
		r, g, b, a := channelsAt(src, i)
		o := i * 8
		putChannel(dst.Pix[o+0:o+2], uint16(uint32(r)*uint32(a)/Max16))
		putChannel(dst.Pix[o+2:o+4], uint16(uint32(g)*uint32(a)/Max16))
		putChannel(dst.Pix[o+4:o+6], uint16(uint32(b)*uint32(a)/Max16))
		putChannel(dst.Pix[o+6:o+8], a)
	}
	return dst
}

func WithAlpha(srcRGB, srcAlpha *image.NRGBA64) *image.NRGBA64 {
	dst := CloneRGBA64(srcRGB)
	if dst == nil {
		return nil
	}
	bounds := dst.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			alpha := alphaAt(srcAlpha, (y-srcAlpha.Rect.Min.Y)*srcAlpha.Rect.Dx()+(x-srcAlpha.Rect.Min.X))
			o := dst.PixOffset(x, y)
			dst.Pix[o+6] = byte(alpha >> 8)
			dst.Pix[o+7] = byte(alpha)
		}
	}
	return dst
}

func AlphaMask(src *image.NRGBA64) *image.Gray16 {
	dst := image.NewGray16(src.Rect)
	n := src.Rect.Dx() * src.Rect.Dy()
	for i := 0; i < n; i++ {
		a := alphaAt(src, i)
		o := i * 2
		dst.Pix[o] = byte(a >> 8)
		dst.Pix[o+1] = byte(a)
	}
	return dst
}

func maxU16(values ...uint16) uint16 {
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func clamp16(value float64) uint16 {
	if value <= 0 {
		return 0
	}
	if value >= Max16 {
		return Max16
	}
	return uint16(math.Round(value))
}

func putChannel(dst []byte, value uint16) {
	dst[0] = byte(value >> 8)
	dst[1] = byte(value)
}
