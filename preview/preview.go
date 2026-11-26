// Package preview composites decoded RGBA/Gray frames over proof backgrounds.
package preview

import (
	"image"
)

// Stats summarizes an alpha plane and black/white proof difference.
type Stats struct {
	Width                 int     `json:"width"`
	Height                int     `json:"height"`
	AlphaMin              int     `json:"alpha_min"`
	AlphaMax              int     `json:"alpha_max"`
	AlphaMean             float64 `json:"alpha_mean"`
	TransparentPixels     int     `json:"transparent_pixels"`
	OpaquePixels          int     `json:"opaque_pixels"`
	PartialPixels         int     `json:"partial_pixels"`
	BlackWhiteMeanDiff    float64 `json:"black_white_mean_diff"`
	PartialEdgePixels     int     `json:"partial_edge_pixels"`
	WhiteHaloLumaDistance float64 `json:"white_halo_luma_distance,omitempty"`
}

// Composite returns straight-alpha composites over black and white.
// rgba must be width*height*4 bytes and alpha width*height bytes.
func Composite(width, height int, rgba, alpha []byte) (black, white *image.RGBA, stats Stats) {
	return composite(width, height, rgba, alpha, false)
}

// CompositeDoublePremultiplication simulates a compositor that receives
// premultiplied color but multiplies it by alpha again as if it were straight.
func CompositeDoublePremultiplication(width, height int, rgba, alpha []byte) (black, white *image.RGBA, stats Stats) {
	return composite(width, height, rgba, alpha, true)
}

func composite(width, height int, rgba, alpha []byte, doublePremultiply bool) (black, white *image.RGBA, stats Stats) {
	black = image.NewRGBA(image.Rect(0, 0, width, height))
	white = image.NewRGBA(image.Rect(0, 0, width, height))
	stats = Stats{Width: width, Height: height, AlphaMin: 255}
	if len(alpha) < width*height || len(rgba) < width*height*4 {
		stats.AlphaMin = 0
		return black, white, stats
	}

	var alphaSum int64
	var diffSum int64
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := y*width + x
			a := alpha[p]
			alphaSum += int64(a)
			if int(a) < stats.AlphaMin {
				stats.AlphaMin = int(a)
			}
			if int(a) > stats.AlphaMax {
				stats.AlphaMax = int(a)
			}
			switch {
			case a == 0:
				stats.TransparentPixels++
			case a == 255:
				stats.OpaquePixels++
			default:
				stats.PartialPixels++
			}

			i := p * 4
			ar := float64(a) / 255.0
			for ch := 0; ch < 3; ch++ {
				src := float64(rgba[i+ch])
				if doublePremultiply {
					src *= ar
				}
				covered := src * ar
				bv := clampByte(covered)
				wv := clampByte(covered + 255*(1-ar))
				black.Pix[i+ch] = bv
				white.Pix[i+ch] = wv
				diffSum += int64(wv) - int64(bv)
			}
			black.Pix[i+3] = 255
			white.Pix[i+3] = 255
		}
	}
	pixels := width * height
	stats.AlphaMean = float64(alphaSum) / float64(pixels)
	stats.BlackWhiteMeanDiff = float64(diffSum) / float64(pixels*3)
	stats.PartialEdgePixels = countPartialEdges(width, height, alpha)
	stats.WhiteHaloLumaDistance = estimateWhiteHalo(width, height, rgba, alpha, white)
	return black, white, stats
}

// CropEdges removes encoded pixels from each edge.
type CropEdges struct {
	Left, Right, Top, Bottom int
}

// Crop returns a cropped copy. Invalid rectangles return the source unchanged.
func Crop(src *image.RGBA, crop CropEdges) *image.RGBA {
	b := src.Bounds()
	r := image.Rect(b.Min.X+crop.Left, b.Min.Y+crop.Top, b.Max.X-crop.Right, b.Max.Y-crop.Bottom)
	if r.Dx() <= 0 || r.Dy() <= 0 || !r.In(b) {
		return src
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := src.PixOffset(x, y)
			o := out.PixOffset(x-r.Min.X, y-r.Min.Y)
			copy(out.Pix[o:o+4], src.Pix[i:i+4])
		}
	}
	return out
}

func countPartialEdges(w, h int, alpha []byte) int {
	count := 0
	for y := 1; y+1 < h; y++ {
		for x := 1; x+1 < w; x++ {
			p := y*w + x
			if alpha[p] == 0 || alpha[p] == 255 {
				continue
			}
			if alpha[p-1] == 0 || alpha[p+1] == 0 || alpha[p-w] == 0 || alpha[p+w] == 0 {
				count++
			}
		}
	}
	return count
}

func estimateWhiteHalo(w, h int, rgba, alpha []byte, white *image.RGBA) float64 {
	var sum float64
	var n int
	for y := 1; y+1 < h; y++ {
		for x := 1; x+1 < w; x++ {
			p := y*w + x
			a := alpha[p]
			if a == 0 || a == 255 {
				continue
			}
			neighborEdge := alpha[p-1] == 0 || alpha[p+1] == 0 || alpha[p-w] == 0 || alpha[p+w] == 0 ||
				alpha[p-1] == 255 || alpha[p+1] == 255 || alpha[p-w] == 255 || alpha[p+w] == 255
			if !neighborEdge {
				continue
			}
			i := p * 4
			srcLuma := 0.2126*float64(rgba[i]) + 0.7152*float64(rgba[i+1]) + 0.0722*float64(rgba[i+2])
			expected := srcLuma*float64(a)/255 + 255*(1-float64(a)/255)
			var r, g, b float64
			rr, gg, bb, _ := white.At(x, y).RGBA()
			r, g, b = float64(rr>>8), float64(gg>>8), float64(bb>>8)
			got := 0.2126*r + 0.7152*g + 0.0722*b
			sum += got - expected
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func clampByte(v float64) byte {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(v + 0.5)
}
