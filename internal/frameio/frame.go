package frameio

import (
	"image"
	"image/color"
	_ "image/png"
	"os"
)

// Frame holds a single image in normalized [0,1] float space together with
// its luma channel. Gray is true when the source is grayscale.
type Frame struct {
	Path   string
	Width  int
	Height int
	Gray   bool
	R      []float64
	G      []float64
	B      []float64
	Y      []float64
}

func scale8(v uint32) float64 {
	return float64(v>>8) / 255.0
}

// Load decodes a PNG file (or a plain PGM fallback) into a Frame.
func Load(path string) (*Frame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return tryPGM(path)
	}
	return fromImage(path, img), nil
}

func fromImage(path string, img image.Image) *Frame {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	n := w * h
	fr := &Frame{Path: path, Width: w, Height: h, R: make([]float64, n), G: make([]float64, n), B: make([]float64, n), Y: make([]float64, n)}
	switch img.ColorModel() {
	case color.GrayModel, color.Gray16Model:
		fr.Gray = true
	}
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			rv, gv, bv := scale8(r), scale8(g), scale8(bl)
			fr.R[i], fr.G[i], fr.B[i] = rv, gv, bv
			fr.Y[i] = 0.2126*rv + 0.7152*gv + 0.0722*bv
			i++
		}
	}
	return fr
}
