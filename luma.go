package tearcheck

import (
	"image"
	"math"
)

// LumaFrame is a compact 8-bit grayscale frame.
type LumaFrame struct {
	Index  int
	PTS    float64
	Width  int
	Height int
	Stride int
	Pix    []byte
}

// LumaFromImage converts a standard image to compact luma.
func LumaFromImage(img image.Image, index int, pts float64) *LumaFrame {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	frame := &LumaFrame{
		Index:  index,
		PTS:    pts,
		Width:  width,
		Height: height,
		Stride: width,
		Pix:    make([]byte, width*height),
	}

	switch src := img.(type) {
	case *image.Gray:
		for y := 0; y < height; y++ {
			srcOff := src.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			dstOff := y * width
			copy(frame.Pix[dstOff:dstOff+width], src.Pix[srcOff:srcOff+width])
		}
		return frame
	case *image.NRGBA:
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				i := src.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)
				r, g, b := float64(src.Pix[i]), float64(src.Pix[i+1]), float64(src.Pix[i+2])
				frame.Pix[y*width+x] = byte(math.Round(0.299*r + 0.587*g + 0.114*b))
			}
		}
		return frame
	case *image.RGBA:
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				i := src.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)
				r, g, b := float64(src.Pix[i]), float64(src.Pix[i+1]), float64(src.Pix[i+2])
				frame.Pix[y*width+x] = byte(math.Round(0.299*r + 0.587*g + 0.114*b))
			}
		}
		return frame
	case *image.YCbCr:
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				frame.Pix[y*width+x] = src.Y[src.YOffset(bounds.Min.X+x, bounds.Min.Y+y)]
			}
		}
		return frame
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			r8 := float64(r >> 8)
			g8 := float64(g >> 8)
			b8 := float64(b >> 8)
			frame.Pix[y*width+x] = byte(math.Round(0.299*r8 + 0.587*g8 + 0.114*b8))
		}
	}
	return frame
}

func (f *LumaFrame) offset(x, y int) int {
	return y*f.Stride + x
}

func absByteDiff(a, b byte) float64 {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return float64(d)
}

// rowBoundaryStats returns mean absolute difference and high-edge coverage for
// the boundary immediately above row y.
func rowBoundaryStats(frame *LumaFrame, y int) (mean, coverage float64) {
	if y <= 0 || y >= frame.Height {
		return 0, 0
	}
	var sum float64
	var strong int
	threshold := 26.0
	for x := 0; x < frame.Width; x++ {
		diff := absByteDiff(frame.Pix[frame.offset(x, y)], frame.Pix[frame.offset(x, y-1)])
		sum += diff
		if diff >= threshold {
			strong++
		}
	}
	n := float64(frame.Width)
	return sum / n, float64(strong) / n
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64{}, values...)
	for i := 0; i < len(cp); i++ {
		for j := i + 1; j < len(cp); j++ {
			if cp[j] < cp[i] {
				cp[i], cp[j] = cp[j], cp[i]
			}
		}
	}
	return cp[len(cp)/2]
}

type seamCandidate struct {
	y          int
	energy     float64
	coverage   float64
	mad        float64
	blockAlign float64
}

// DetectSeams finds non-maximum-suppressed horizontal discontinuity rows.
func DetectSeams(frame *LumaFrame, blockSizes []int) []seamCandidate {
	if frame == nil || frame.Width < 16 || frame.Height < 16 {
		return nil
	}
	mads := make([]float64, frame.Height)
	type rowStat struct {
		y        int
		mad      float64
		coverage float64
	}
	stats := make([]rowStat, 0, frame.Height-2)
	allMAD := make([]float64, 0, frame.Height-1)
	for y := 1; y < frame.Height; y++ {
		mad, coverage := rowBoundaryStats(frame, y)
		mads[y] = mad
		allMAD = append(allMAD, mad)
		stats = append(stats, rowStat{y: y, mad: mad, coverage: coverage})
	}
	baseline := medianFloat(allMAD)

	candidates := make([]seamCandidate, 0, 6)
	for _, stat := range stats {
		y := stat.y
		if y < 3 || y > frame.Height-4 {
			continue
		}
		energy := Clamp01((stat.mad - 8) / 34)
		relative := baseline > 2 && stat.mad < baseline*1.6
		if energy < 0.34 || stat.coverage < 0.30 || relative {
			continue
		}
		if y > 1 && mads[y] < mads[y-1] {
			// Keep local maxima, allowing a one-row plateau.
			if y+1 < frame.Height && mads[y] < mads[y+1] {
				continue
			}
		}
		align := BlockAlignmentAt(y, blockSizes)
		candidates = append(candidates, seamCandidate{
			y:          y,
			energy:     energy,
			coverage:   stat.coverage,
			mad:        stat.mad,
			blockAlign: align,
		})
	}

	// Non-maximum suppression within four scanlines.
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if absInt(candidates[j].y-candidates[i].y) <= 4 && candidates[j].energy > candidates[i].energy {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	kept := candidates[:0]
	for _, candidate := range candidates {
		duplicate := false
		for _, prior := range kept {
			if absInt(prior.y-candidate.y) <= 4 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			kept = append(kept, candidate)
		}
		if len(kept) >= 6 {
			break
		}
	}
	return kept
}

// BlockAlignmentAt returns how plausibly y is a horizontal codec block edge.
func BlockAlignmentAt(y int, sizes []int) float64 {
	if len(sizes) == 0 {
		sizes = DefaultBlockSizes("")
	}
	best := 0.0
	for _, size := range sizes {
		if size <= 1 {
			continue
		}
		rem := y % size
		distance := rem
		if size-rem < distance {
			distance = size - rem
		}
		score := Clamp01(1 - float64(distance)/2.0)
		switch {
		case size <= 4:
			score *= 0.45
		case size == 8:
			score *= 0.70
		case size >= 16:
			score *= 1.0
		}
		if score > best {
			best = score
		}
	}
	return best
}

// DefaultBlockSizes returns likely horizontal block sizes for a codec name.
func DefaultBlockSizes(codec string) []int {
	switch codec {
	case "h264", "avc", "h264_nvenc":
		return []int{16, 8, 4}
	case "hevc", "h265", "hevc_nvenc":
		return []int{64, 32, 16, 8, 4}
	case "av1":
		return []int{64, 32, 16, 8, 4}
	case "vp9":
		return []int{64, 32, 16, 8, 4}
	default:
		return []int{16, 8, 4, 32, 64}
	}
}
