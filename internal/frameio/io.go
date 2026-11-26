package frameio

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var supported = map[string]bool{".png": true, ".pgm": true, ".ppm": true}

// ListFrames returns sorted supported image paths inside dir (non-recursive).
func ListFrames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if supported[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// SavePNG writes the frame back as 8-bit PNG.
func (f *Frame) SavePNG(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
	i := 0
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			o := img.PixOffset(x, y)
			img.Pix[o+0] = clamp8(f.R[i])
			img.Pix[o+1] = clamp8(f.G[i])
			img.Pix[o+2] = clamp8(f.B[i])
			img.Pix[o+3] = 255
			i++
		}
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, img)
}

func clamp8(v float64) uint8 {
	v = v*255 + 0.5
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// tryPGM reads binary (P5) or ASCII (P2) PGM and P6 PPM.
func tryPGM(path string) (*Frame, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := &pnmReader{data: data}
	magic := r.token()
	if magic != "P5" && magic != "P2" && magic != "P6" && magic != "P3" {
		return nil, fmt.Errorf("unsupported image: %s", path)
	}
	w, h, maxv := r.intToken(), r.intToken(), r.intToken()
	if w <= 0 || h <= 0 || maxv <= 0 || maxv > 65535 {
		return nil, fmt.Errorf("bad pnm header: %s", path)
	}
	n := w * h
	fr := &Frame{Path: path, Width: w, Height: h, R: make([]float64, n), G: make([]float64, n), B: make([]float64, n), Y: make([]float64, n)}
	gray := magic == "P5" || magic == "P2"
	fr.Gray = gray
	scale := 1.0 / float64(maxv)
	if magic == "P5" || magic == "P6" {
		r.skipSingleWhitespace()
		bpp := 1
		if maxv > 255 {
			bpp = 2
		}
		components := 1
		if magic == "P6" {
			components = 3
		}
		for i := 0; i < n*components; i++ {
			v := 0
			for j := 0; j < bpp; j++ {
				if r.pos >= len(r.data) {
					return nil, fmt.Errorf("truncated pnm: %s", path)
				}
				v = v<<8 | int(r.data[r.pos])
				r.pos++
			}
			fv := float64(v) * scale
			c := i % components
			idx := i / components
			if gray {
				fr.R[idx], fr.G[idx], fr.B[idx] = fv, fv, fv
			} else if c == 0 {
				fr.R[idx] = fv
			} else if c == 1 {
				fr.G[idx] = fv
			} else {
				fr.B[idx] = fv
			}
		}
	} else {
		for i := 0; i < n; i++ {
			if gray {
				v := float64(r.intToken()) * scale
				fr.R[i], fr.G[i], fr.B[i] = v, v, v
			} else {
				fr.R[i] = float64(r.intToken()) * scale
				fr.G[i] = float64(r.intToken()) * scale
				fr.B[i] = float64(r.intToken()) * scale
			}
		}
	}
	for i := range fr.Y {
		fr.Y[i] = 0.2126*fr.R[i] + 0.7152*fr.G[i] + 0.0722*fr.B[i]
	}
	return fr, nil
}

type pnmReader struct {
	data []byte
	pos  int
}

func (r *pnmReader) skipByte(isWS func(byte) bool) {
	for r.pos < len(r.data) && isWS(r.data[r.pos]) {
		r.pos++
	}
}

func (r *pnmReader) skipCommentsAndSpace() {
	for r.pos < len(r.data) {
		c := r.data[r.pos]
		if c == '#' {
			for r.pos < len(r.data) && r.data[r.pos] != '\n' {
				r.pos++
			}
			continue
		}
		if isWhitespace(c) {
			r.pos++
			continue
		}
		return
	}
}

func (r *pnmReader) token() string {
	r.skipCommentsAndSpace()
	start := r.pos
	for r.pos < len(r.data) && !isWhitespace(r.data[r.pos]) {
		r.pos++
	}
	return string(r.data[start:r.pos])
}

func (r *pnmReader) intToken() int {
	v, _ := strconv.Atoi(r.token())
	return v
}

func (r *pnmReader) skipSingleWhitespace() {
	if r.pos < len(r.data) && isWhitespace(r.data[r.pos]) {
		r.pos++
	}
}

func isWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}
