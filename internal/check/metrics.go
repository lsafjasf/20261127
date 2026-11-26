package check

import (
	"image"
	"math"

	"proalpha/internal/imagex"
)

func MeanAbsError(a, b *image.NRGBA64) FrameMetrics {
	bounds := a.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	metrics := FrameMetrics{Pixels: width * height}
	if metrics.Pixels == 0 || !b.Bounds().Eq(bounds) {
		return metrics
	}

	var sumR, sumG, sumB, sumA float64
	for i := 0; i < metrics.Pixels; i++ {
		ao := i * 8
		bo := i * 8
		ar := float64(uint16(a.Pix[ao+0])<<8 | uint16(a.Pix[ao+1]))
		ag := float64(uint16(a.Pix[ao+2])<<8 | uint16(a.Pix[ao+3]))
		ab := float64(uint16(a.Pix[ao+4])<<8 | uint16(a.Pix[ao+5]))
		aa := float64(uint16(a.Pix[ao+6])<<8 | uint16(a.Pix[ao+7]))
		br := float64(uint16(b.Pix[bo+0])<<8 | uint16(b.Pix[bo+1]))
		bg := float64(uint16(b.Pix[bo+2])<<8 | uint16(b.Pix[bo+3]))
		bb := float64(uint16(b.Pix[bo+4])<<8 | uint16(b.Pix[bo+5]))
		ba := float64(uint16(b.Pix[bo+6])<<8 | uint16(b.Pix[bo+7]))
		sumR += math.Abs(ar - br)
		sumG += math.Abs(ag - bg)
		sumB += math.Abs(ab - bb)
		sumA += math.Abs(aa - ba)
	}
	n := float64(metrics.Pixels)
	metrics.RMeanAbsError = sumR / n
	metrics.GMeanAbsError = sumG / n
	metrics.BMeanAbsError = sumB / n
	metrics.AMeanAbsError = sumA / n
	metrics.RGBMeanAbsError = (sumR + sumG + sumB) / (3 * n)
	return metrics
}

func CompareAlphaModel(reference, candidate *image.NRGBA64) ModelComparison {
	width := reference.Bounds().Dx()
	height := reference.Bounds().Dy()
	pixels := width * height
	var straightSum, premulSum, alphaSum float64
	var counted int

	for i := 0; i < pixels; i++ {
		ro := i * 8
		co := i * 8
		rr := float64(uint16(reference.Pix[ro+0])<<8 | uint16(reference.Pix[ro+1]))
		rg := float64(uint16(reference.Pix[ro+2])<<8 | uint16(reference.Pix[ro+3]))
		rb := float64(uint16(reference.Pix[ro+4])<<8 | uint16(reference.Pix[ro+5]))
		ra := float64(uint16(reference.Pix[ro+6])<<8 | uint16(reference.Pix[ro+7]))
		cr := float64(uint16(candidate.Pix[co+0])<<8 | uint16(candidate.Pix[co+1]))
		cg := float64(uint16(candidate.Pix[co+2])<<8 | uint16(candidate.Pix[co+3]))
		cb := float64(uint16(candidate.Pix[co+4])<<8 | uint16(candidate.Pix[co+5]))
		ca := float64(uint16(candidate.Pix[co+6])<<8 | uint16(candidate.Pix[co+7]))
		alphaSum += math.Abs(ra - ca)

		if ra < float64(imagex.Max16)*0.03 {
			continue
		}
		counted++
		straightSum += math.Abs(rr-cr) + math.Abs(rg-cg) + math.Abs(rb-cb)
		pr := rr * ra / float64(imagex.Max16)
		pg := rg * ra / float64(imagex.Max16)
		pb := rb * ra / float64(imagex.Max16)
		premulSum += math.Abs(pr-cr) + math.Abs(pg-cg) + math.Abs(pb-cb)
	}

	comparison := ModelComparison{Winner: "indeterminate"}
	if counted > 0 {
		comparison.StraightRGBError = straightSum / float64(3*counted)
		comparison.PremultipliedError = premulSum / float64(3*counted)
	}
	if pixels > 0 {
		comparison.AlphaError = alphaSum / float64(pixels)
	}

	const minAdvantage = 900.0
	if comparison.PremultipliedError+minAdvantage < comparison.StraightRGBError &&
		comparison.PremultipliedError < 9000 {
		comparison.Winner = "premultiplied"
	} else if comparison.StraightRGBError+minAdvantage < comparison.PremultipliedError &&
		comparison.StraightRGBError < 9000 {
		comparison.Winner = "straight"
	}
	return comparison
}
