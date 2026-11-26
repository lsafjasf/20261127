package flatfield

import (
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// ImageRaster adapts image.Image to Raster and linearizes sRGB samples.
type ImageRaster struct {
	Image image.Image
}

// Size implements Raster.
func (r ImageRaster) Size() (int, int) {
	b := r.Image.Bounds()
	return b.Dx(), b.Dy()
}

// LinearRGBAt implements Raster.
func (r ImageRaster) LinearRGBAt(x, y int) RGB {
	b := r.Image.Bounds()
	c := color.NRGBA64Model.Convert(r.Image.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA64)
	return RGB{
		R: srgbToLinear(float64(c.R) / 65535.0),
		G: srgbToLinear(float64(c.G) / 65535.0),
		B: srgbToLinear(float64(c.B) / 65535.0),
	}
}

// LoadImage decodes a PNG, JPEG or GIF image.
func LoadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// LoadRaster loads and adapts an image to the linear Raster interface.
func LoadRaster(path string) (Raster, error) {
	img, err := LoadImage(path)
	if err != nil {
		return nil, err
	}
	return ImageRaster{Image: img}, nil
}

// SaveImage encodes img as PNG or JPEG based on the output extension.
func SaveImage(path string, img image.Image, jpegQuality int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		enc := png.Encoder{CompressionLevel: png.DefaultCompression}
		return enc.Encode(f, img)
	case ".jpg", ".jpeg":
		q := jpegQuality
		if q <= 0 {
			q = 95
		}
		return jpeg.Encode(f, img, &jpeg.Options{Quality: q})
	default:
		if ext == "" {
			return png.Encode(f, img)
		}
		q := jpegQuality
		if q <= 0 {
			q = 95
		}
		return jpeg.Encode(f, img, &jpeg.Options{Quality: q})
	}
}

// CorrectImage applies the table in linear RGB and returns a 16-bit PNG-ready
// NRGBA64 image. Alpha is preserved.
func CorrectImage(img image.Image, t *Table) (*image.NRGBA64, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := (float64(y) + 0.5) / float64(h)
		for x := 0; x < w; x++ {
			u := (float64(x) + 0.5) / float64(w)
			gain := t.GainAt(u, v)
			c := color.NRGBA64Model.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA64)
			lin := RGB{
				R: srgbToLinear(float64(c.R) / 65535),
				G: srgbToLinear(float64(c.G) / 65535),
				B: srgbToLinear(float64(c.B) / 65535),
			}
			corrected := ApplyGain(lin, gain)
			peak := math.Max(corrected.R, math.Max(corrected.G, corrected.B))
			if peak > 1 {
				guard := 1.0 / peak
				corrected = ApplyGain(corrected, guard)
			}
			out.SetNRGBA64(x, y, color.NRGBA64{
				R: clampNRGBA64(srgbFromLinear(corrected.R)),
				G: clampNRGBA64(srgbFromLinear(corrected.G)),
				B: clampNRGBA64(srgbFromLinear(corrected.B)),
				A: c.A,
			})
		}
	}
	return out, nil
}

func clampNRGBA64(v float64) uint16 {
	v = clampFloat(v, 0, 1) * 65535
	return uint16(v + 0.5)
}

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func srgbFromLinear(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1.0/2.4) - 0.055
}
