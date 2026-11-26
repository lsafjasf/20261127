package analyzer

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
}

func expandPaths(inputs []string) ([]string, error) {
	var paths []string
	for _, input := range inputs {
		for _, part := range strings.Split(input, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			info, err := os.Stat(part)
			if err != nil {
				return nil, fmt.Errorf("stat %q: %w", part, err)
			}
			if !info.IsDir() {
				paths = append(paths, part)
				continue
			}
			entries, err := os.ReadDir(part)
			if err != nil {
				return nil, fmt.Errorf("read directory %q: %w", part, err)
			}
			var found []string
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if imageExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
					found = append(found, filepath.Join(part, entry.Name()))
				}
			}
			naturalSort(found)
			paths = append(paths, found...)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no PNG, JPEG, or GIF frames found")
	}
	for _, path := range paths {
		if !imageExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil, fmt.Errorf("unsupported frame %q: use PNG, JPEG, or GIF", path)
		}
	}
	return paths, nil
}

func naturalSort(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		return naturalLess(filepath.Base(paths[i]), filepath.Base(paths[j]))
	})
}

func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		ar, an := nextNaturalPart(a)
		br, bn := nextNaturalPart(b)
		if an && bn {
			ai, _ := strconv.ParseUint(ar, 10, 64)
			bi, _ := strconv.ParseUint(br, 10, 64)
			if ai != bi {
				return ai < bi
			}
			if len(ar) != len(br) {
				return len(ar) < len(br)
			}
		} else if ar != br {
			return ar < br
		}
		a = a[len(ar):]
		b = b[len(br):]
	}
	return len(a) < len(b)
}

func nextNaturalPart(s string) (string, bool) {
	asDigit := unicode.IsDigit(rune(s[0]))
	i := 1
	for i < len(s) && unicode.IsDigit(rune(s[i])) == asDigit {
		i++
	}
	return s[:i], asDigit
}

func decodeLuma(path string) (*plane, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}
	return planeFromImage(img), nil
}

func decodeSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, fmt.Errorf("decode config %q: %w", path, err)
	}
	return config.Width, config.Height, nil
}

func planeFromImage(img image.Image) *plane {
	b := img.Bounds()
	out := &plane{W: b.Dx(), H: b.Dy(), P: make([]float32, b.Dx()*b.Dy())}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			// RGBA returns premultiplied values. Treat transparency as black,
			// which is deterministic and preserves alpha-weighted edge energy.
			yOffset := y - b.Min.Y
			xOffset := x - b.Min.X
			out.P[yOffset*out.W+xOffset] = float32(0.2126*float64(r)/257 + 0.7152*float64(g)/257 + 0.0722*float64(bl)/257)
		}
	}
	return out
}

func cropPlane(src *plane, r Rect) (*plane, error) {
	if r.X < 0 || r.Y < 0 || r.W <= 0 || r.H <= 0 || r.X+r.W > src.W || r.Y+r.H > src.H {
		return nil, fmt.Errorf("ROI %d,%d,%d,%d is outside a %dx%d frame", r.X, r.Y, r.W, r.H, src.W, src.H)
	}
	out := &plane{W: r.W, H: r.H, P: make([]float32, r.W*r.H)}
	for y := 0; y < r.H; y++ {
		copy(out.P[y*r.W:(y+1)*r.W], src.P[(r.Y+y)*src.W+r.X:(r.Y+y)*src.W+r.X+r.W])
	}
	return out, nil
}

func mapROI(r Rect, sx, sy float64, scaledW, scaledH int) (Rect, error) {
	x0 := int(float64(r.X) * sx)
	y0 := int(float64(r.Y) * sy)
	x1 := int(float64(r.X+r.W) * sx)
	y1 := int(float64(r.Y+r.H) * sy)
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	x0 = max(0, min(x0, scaledW-1))
	y0 = max(0, min(y0, scaledH-1))
	x1 = max(x0+1, min(x1, scaledW))
	y1 = max(y0+1, min(y1, scaledH))
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, nil
}
