package rangecheck

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // JPEG decoder registration for image-sequence input.
	_ "image/png"  // PNG decoder registration for image-sequence input.
	"math"
)

// DefaultSettings returns conservative 8-bit limited-range settings.
func DefaultSettings(w, h int) Settings {
	return Settings{
		Range:           RangeTV,
		PixelFormat:     FormatYUV420P,
		Width:           w,
		Height:          h,
		Bits:            8,
		ColorMatrix:     "bt601",
		MinRegionPixels: defaultMinRegionPixels(w, h),
		MaxFrameReports: 0,
	}
}

func defaultMinRegionPixels(w, h int) int {
	n := w * h / 12000
	if n < 16 {
		return 16
	}
	if n > 256 {
		return 256
	}
	return n
}

// LumaBounds returns nominal legal luma bounds for the configured range.
func (s Settings) LumaBounds() LevelBounds {
	if s.Range == RangeFull {
		return LevelBounds{0, 255}
	}
	return LevelBounds{16, 235}
}

// ChromaBounds returns nominal legal chroma bounds.
func (s Settings) ChromaBounds() LevelBounds {
	if s.Range == RangeFull {
		return LevelBounds{0, 255}
	}
	return LevelBounds{16, 240}
}

func (s Settings) withDefaults() Settings {
	if s.Range == "" {
		s.Range = RangeTV
	}
	if s.Bits == 0 {
		s.Bits = 8
	}
	if s.ColorMatrix == "" {
		s.ColorMatrix = "bt601"
	}
	if s.MinRegionPixels <= 0 && s.Width > 0 && s.Height > 0 {
		s.MinRegionPixels = defaultMinRegionPixels(s.Width, s.Height)
	}
	return s
}

// AnalyzeFrame computes statistics and classified regions for one frame.
func AnalyzeFrame(frame Frame, settings Settings) (*FrameReport, error) {
	settings = settings.withDefaults()
	if frame.Width <= 0 || frame.Height <= 0 {
		return nil, fmt.Errorf("frame %d has invalid dimensions %dx%d", frame.Index, frame.Width, frame.Height)
	}
	if settings.Width == 0 {
		settings.Width = frame.Width
	}
	if settings.Height == 0 {
		settings.Height = frame.Height
	}
	if settings.MinRegionPixels <= 0 {
		settings.MinRegionPixels = defaultMinRegionPixels(frame.Width, frame.Height)
	}
	if err := prepareFramePlanes(&frame, settings); err != nil {
		return nil, err
	}

	bounds := settings.LumaBounds()
	yStats := measurePlane("Y", frame.Y, frame.Width, frame.Height, frame.StrideY, bounds)
	report := &FrameReport{
		Index:      frame.Index,
		PTSSeconds: frame.PTSSeconds,
		Width:      frame.Width,
		Height:     frame.Height,
		Y:          yStats,
	}
	if len(frame.Cb) > 0 {
		cb := measurePlane("Cb", frame.Cb, chromaWidth(frame.Width, settings.PixelFormat), chromaHeight(frame.Height, settings.PixelFormat), frame.StrideC, settings.ChromaBounds())
		report.Cb = &cb
	}
	if len(frame.Cr) > 0 {
		cr := measurePlane("Cr", frame.Cr, chromaWidth(frame.Width, settings.PixelFormat), chromaHeight(frame.Height, settings.PixelFormat), frame.StrideC, settings.ChromaBounds())
		report.Cr = &cr
	}

	report.BlackPixels = yStats.BelowLegal
	report.WhitePixels = yStats.AboveLegal
	report.OutOfRangePixels = yStats.BelowLegal + yStats.AboveLegal
	report.OutOfRangeRatio = float64(report.OutOfRangePixels) / float64(frame.Width*frame.Height)
	report.HasOutOfRange = report.OutOfRangePixels > 0

	regions, err := detectRegions(frame, settings, bounds)
	if err != nil {
		return nil, err
	}
	report.Regions = regions
	return report, nil
}

func chromaWidth(w int, format PixelFormat) int {
	if format == FormatYUV420P {
		return (w + 1) / 2
	}
	return w
}

func chromaHeight(h int, format PixelFormat) int {
	if format == FormatYUV420P {
		return (h + 1) / 2
	}
	return h
}

func prepareFramePlanes(frame *Frame, settings Settings) error {
	if frame.StrideY == 0 {
		frame.StrideY = frame.Width
	}
	if frame.Format == "" {
		frame.Format = settings.PixelFormat
	}
	switch frame.Format {
	case FormatGray8:
		if len(frame.Y) < frame.StrideY*frame.Height {
			return fmt.Errorf("frame %d: short gray plane: have %d want at least %d", frame.Index, len(frame.Y), frame.StrideY*frame.Height)
		}
	case FormatYUV420P, FormatYUV444P:
		cw, ch := chromaWidth(frame.Width, frame.Format), chromaHeight(frame.Height, frame.Format)
		if frame.StrideC == 0 {
			frame.StrideC = cw
		}
		if len(frame.Y) < frame.StrideY*frame.Height {
			return fmt.Errorf("frame %d: short Y plane", frame.Index)
		}
		if len(frame.Cb) < frame.StrideC*ch || len(frame.Cr) < frame.StrideC*ch {
			return fmt.Errorf("frame %d: short chroma planes", frame.Index)
		}
	case FormatRGB24, FormatRGBA32:
		if len(frame.RGB) == 0 {
			return fmt.Errorf("frame %d: packed RGB frame has no pixels", frame.Index)
		}
		deriveYFromRGB(frame, settings)
	default:
		return fmt.Errorf("frame %d: unsupported pixel format %q", frame.Index, frame.Format)
	}
	return nil
}

func deriveYFromRGB(frame *Frame, settings Settings) {
	bytesPerPixel := 3
	if frame.Format == FormatRGBA32 {
		bytesPerPixel = 4
	}
	if frame.StrideY == 0 || len(frame.Y) != frame.Width*frame.Height {
		frame.Y = make([]byte, frame.Width*frame.Height)
		frame.StrideY = frame.Width
	}
	limited := settings.Range == RangeTV
	coeffs := matrixCoefficients(settings.ColorMatrix)
	for y := 0; y < frame.Height; y++ {
		for x := 0; x < frame.Width; x++ {
			i := (y*frame.Width + x) * bytesPerPixel
			r, g, b := float64(frame.RGB[i]), float64(frame.RGB[i+1]), float64(frame.RGB[i+2])
			luma := coeffs.r*r + coeffs.g*g + coeffs.b*b
			var code float64
			if limited {
				code = 16 + 219*luma/255
			} else {
				code = luma
			}
			frame.Y[y*frame.StrideY+x] = byte(clampInt(int(math.Round(code)), 0, 255))
		}
	}
}

type lumaMatrix struct{ r, g, b float64 }

func matrixCoefficients(name string) lumaMatrix {
	switch name {
	case "bt709", "rec709":
		return lumaMatrix{0.2126, 0.7152, 0.0722}
	case "bt2020", "rec2020":
		return lumaMatrix{0.2627, 0.6780, 0.0593}
	default:
		return lumaMatrix{0.299, 0.587, 0.114}
	}
}

func measurePlane(name string, plane []byte, w, h, stride int, bounds LevelBounds) PlaneStats {
	if stride <= 0 {
		stride = w
	}
	stats := PlaneStats{Plane: name, Pixels: w * h, Min: 256, Max: -1}
	var sum int64
	for y := 0; y < h; y++ {
		row := y * stride
		for x := 0; x < w; x++ {
			v := int(plane[row+x])
			sum += int64(v)
			if v < stats.Min {
				stats.Min = v
			}
			if v > stats.Max {
				stats.Max = v
			}
			if v < bounds.Black {
				stats.BelowLegal++
			} else if v > bounds.White {
				stats.AboveLegal++
			} else {
				stats.WithinLegal++
			}
			if v <= 12 {
				stats.BelowSoftEdge++
			}
			if v >= 240 {
				stats.AboveSoftEdge++
			}
			if v == 0 {
				stats.ExactBlack++
			}
			if v == 255 {
				stats.ExactWhite++
			}
			if v >= bounds.Black && v <= bounds.Black+2 {
				stats.NearBlackLegal++
			}
			if v >= bounds.White-2 && v <= bounds.White {
				stats.NearWhiteLegal++
			}
		}
	}
	if stats.Pixels > 0 {
		stats.Mean = float64(sum) / float64(stats.Pixels)
		stats.BelowRatio = float64(stats.BelowLegal) / float64(stats.Pixels)
		stats.AboveRatio = float64(stats.AboveLegal) / float64(stats.Pixels)
		stats.OutOfRangeRatio = stats.BelowRatio + stats.AboveRatio
	}
	if stats.Min == 256 {
		stats.Min, stats.Max = 0, 0
	}
	return stats
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// DecodeImageToFrame converts an image.Image to packed RGBA Frame data.
func DecodeImageToFrame(img image.Image, index int, pts float64) Frame {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	rgb := make([]byte, w*h*4)
	copy(rgb, rgba.Pix)
	return Frame{
		Index: index, PTSSeconds: pts, Width: w, Height: h,
		Format: FormatRGBA32, RGB: rgb,
	}
}
