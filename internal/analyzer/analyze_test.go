package analyzer

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnosesSyntheticCases(t *testing.T) {
	tests := []struct {
		name string
		gen  func(t *testing.T, dir string) ([]string, []string)
		want Verdict
	}{
		{name: "bilinear display sampling", gen: makeBilinearDisplayCase, want: VerdictDisplaySampling},
		{name: "display sampling", gen: makeDisplayCase, want: VerdictDisplaySampling},
		{name: "encoding", gen: makeEncodingCase, want: VerdictEncoding},
		{name: "temporal encoding", gen: makeTemporalEncodingCase, want: VerdictEncoding},
		{name: "blurry source", gen: makeBlurrySourceCase, want: VerdictSource},
		{name: "blocky source", gen: makeSourceBlockCase, want: VerdictSource},
		{name: "flat frames", gen: makeFlatCase, want: VerdictInconclusive},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sources, scaled := tc.gen(t, dir)
			report, err := Analyze(Options{
				SourcePaths: sources,
				ScaledPaths: scaled,
				BlockSizes:  []int{8, 16},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("verdict=%s confidence=%.2f comparison=%+v", report.Verdict, report.Confidence, report.Comparison)
			t.Logf("source=%+v", report.Source)
			t.Logf("scaled=%+v", report.Scaled)
			if report.Verdict != tc.want {
				t.Fatalf("verdict = %s, want %s; evidence: %+v", report.Verdict, tc.want, report.Evidence)
			}
		})
	}
}

func TestDetectBlockGrid(t *testing.T) {
	p := &plane{W: 128, H: 96, P: make([]float32, 128*96)}
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W; x++ {
			v := 128.0 + 0.03*float64(x)
			if x%8 == 7 {
				v += 6
			}
			p.P[y*p.W+x] = float32(v)
		}
	}
	blocks := analyzeBlocks(p, []int{8, 16, 32})
	if blocks.Best.Size != 8 || blocks.Best.Axis != "horizontal" || (blocks.Best.Phase != 6 && blocks.Best.Phase != 7) {
		t.Fatalf("best = %+v, want 8px horizontal boundary phase 6/7", blocks.Best)
	}
	if blocks.Best.Excess < 0.5 {
		t.Fatalf("block excess = %.3f, want a clear boundary", blocks.Best.Excess)
	}
}

func makeDisplayCase(t *testing.T, dir string) ([]string, []string) {
	t.Helper()
	base := textPlane(360, 240, 1.0)
	var sources, scaled []string
	for frame := 0; frame < 3; frame++ {
		sources = append(sources, writeTestPNG(t, filepath.Join(dir, "src_display_"+itoa(frame)+".png"), base))
		up := resizeBilinear(base, 540, 360)
		soft := gaussianBlur(up, 1.15)
		scaled = append(scaled, writeTestPNG(t, filepath.Join(dir, "scaled_display_"+itoa(frame)+".png"), soft))
	}
	return sources, scaled
}

func makeBilinearDisplayCase(t *testing.T, dir string) ([]string, []string) {
	t.Helper()
	base := textPlane(360, 240, 1.0)
	var sources, scaled []string
	for frame := 0; frame < 3; frame++ {
		sources = append(sources, writeTestPNG(t, filepath.Join(dir, "src_bilinear_"+itoa(frame)+".png"), base))
		up := resizeBilinear(base, 540, 360)
		scaled = append(scaled, writeTestPNG(t, filepath.Join(dir, "scaled_bilinear_"+itoa(frame)+".png"), up))
	}
	return sources, scaled
}

func makeEncodingCase(t *testing.T, dir string) ([]string, []string) {
	t.Helper()
	base := textPlane(360, 240, 1.0)
	var sources, scaled []string
	for frame := 0; frame < 3; frame++ {
		sources = append(sources, writeTestPNG(t, filepath.Join(dir, "src_encoding_"+itoa(frame)+".png"), base))
		up := resizeNearest(base, 540, 360)
		addCodecBlocks(up, frame)
		scaled = append(scaled, writeTestPNG(t, filepath.Join(dir, "scaled_encoding_"+itoa(frame)+".png"), up))
	}
	return sources, scaled
}

func makeBlurrySourceCase(t *testing.T, dir string) ([]string, []string) {
	t.Helper()
	base := gaussianBlur(textPlane(360, 240, 1.0), 1.45)
	var sources, scaled []string
	for frame := 0; frame < 3; frame++ {
		sources = append(sources, writeTestPNG(t, filepath.Join(dir, "src_blur_"+itoa(frame)+".png"), base))
		up := resizeBilinear(base, 540, 360)
		scaled = append(scaled, writeTestPNG(t, filepath.Join(dir, "scaled_blur_"+itoa(frame)+".png"), up))
	}
	return sources, scaled
}

func makeTemporalEncodingCase(t *testing.T, dir string) ([]string, []string) {
	t.Helper()
	base := textPlane(360, 240, 1.0)
	var sources, scaled []string
	for frame := 0; frame < 3; frame++ {
		sources = append(sources, writeTestPNG(t, filepath.Join(dir, "src_temporal_"+itoa(frame)+".png"), base))
		up := resizeBilinear(base, 540, 360)
		for i := range up.P {
			if (i*1103515245+frame*12345)%11 == 0 {
				up.P[i] = float32(clamp(float64(up.P[i])+float64(frame-1)*4, 0, 255))
			}
		}
		scaled = append(scaled, writeTestPNG(t, filepath.Join(dir, "scaled_temporal_"+itoa(frame)+".png"), up))
	}
	return sources, scaled
}

func makeFlatCase(t *testing.T, dir string) ([]string, []string) {
	t.Helper()
	base := &plane{W: 96, H: 64, P: make([]float32, 96*64)}
	for i := range base.P {
		base.P[i] = 128
	}
	var sources, scaled []string
	for frame := 0; frame < 3; frame++ {
		sources = append(sources, writeTestPNG(t, filepath.Join(dir, "src_flat_"+itoa(frame)+".png"), base))
		scaled = append(scaled, writeTestPNG(t, filepath.Join(dir, "scaled_flat_"+itoa(frame)+".png"), base))
	}
	return sources, scaled
}

func makeSourceBlockCase(t *testing.T, dir string) ([]string, []string) {
	t.Helper()
	base := textPlane(360, 240, 1.0)
	addCodecBlocks(base, 0)
	up := resizeNearest(base, 540, 360)
	var sources, scaled []string
	for frame := 0; frame < 3; frame++ {
		sources = append(sources, writeTestPNG(t, filepath.Join(dir, "src_block_"+itoa(frame)+".png"), base))
		scaled = append(scaled, writeTestPNG(t, filepath.Join(dir, "scaled_block_"+itoa(frame)+".png"), up))
	}
	return sources, scaled
}

func textPlane(w, h int, phase float64) *plane {
	p := &plane{W: w, H: h, P: make([]float32, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p.P[y*w+x] = 235
		}
	}
	for i := 0; i < 13; i++ {
		x := 18 + i*23 + int(3*math.Sin(float64(i)*2+phase))
		y := 26 + (i%5)*38
		drawRect(p, x, y, 3+(i%3), 23-(i%4), 28)
		drawRect(p, x+3, y+8, 11, 3, 28)
	}
	for i := 0; i < 6; i++ {
		x := 35 + i*49
		y := 205 - i*3
		drawRect(p, x, y, 31, 2, 18)
	}
	return p
}

func drawRect(p *plane, x0, y0, w, h int, value float32) {
	for y := y0; y < min(y0+h, p.H); y++ {
		for x := x0; x < min(x0+w, p.W); x++ {
			if x >= 0 && y >= 0 {
				p.P[y*p.W+x] = value
			}
		}
	}
}

func resizeBilinear(src *plane, w, h int) *plane {
	dst := &plane{W: w, H: h, P: make([]float32, w*h)}
	sx := float64(src.W) / float64(w)
	sy := float64(src.H) / float64(h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.P[y*w+x] = float32(sampleBilinear(src, (float64(x)+0.5)*sx-0.5, (float64(y)+0.5)*sy-0.5))
		}
	}
	return dst
}

func resizeNearest(src *plane, w, h int) *plane {
	dst := &plane{W: w, H: h, P: make([]float32, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := min(src.W-1, int(float64(x)*float64(src.W)/float64(w)))
			sy := min(src.H-1, int(float64(y)*float64(src.H)/float64(h)))
			dst.P[y*w+x] = src.P[sy*src.W+sx]
		}
	}
	return dst
}

func gaussianBlur(src *plane, sigma float64) *plane {
	radius := int(math.Ceil(3 * sigma))
	kernel := make([]float64, 2*radius+1)
	var sum float64
	for i := -radius; i <= radius; i++ {
		v := math.Exp(-float64(i*i) / (2 * sigma * sigma))
		kernel[i+radius] = v
		sum += v
	}
	for i := range kernel {
		kernel[i] /= sum
	}
	tmp := &plane{W: src.W, H: src.H, P: make([]float32, len(src.P))}
	dst := &plane{W: src.W, H: src.H, P: make([]float32, len(src.P))}
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			var v float64
			for k := -radius; k <= radius; k++ {
				v += kernel[k+radius] * src.at(clampInt(x+k, 0, src.W-1), y)
			}
			tmp.P[y*src.W+x] = float32(v)
		}
	}
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			var v float64
			for k := -radius; k <= radius; k++ {
				v += kernel[k+radius] * tmp.at(x, clampInt(y+k, 0, src.H-1))
			}
			dst.P[y*src.W+x] = float32(v)
		}
	}
	return dst
}

func addCodecBlocks(p *plane, frame int) {
	for by := 0; by < p.H; by += 8 {
		for bx := 0; bx < p.W; bx += 8 {
			shift := float32(((bx/8*13+by/8*7+frame*3)%5)-2) * 0.9
			for y := by; y < min(by+8, p.H); y++ {
				for x := bx; x < min(bx+8, p.W); x++ {
					if p.P[y*p.W+x] > 210 {
						p.P[y*p.W+x] = float32(clamp(float64(p.P[y*p.W+x]+shift), 0, 255))
					}
				}
			}
			if bx+7 < p.W {
				for y := by; y < min(by+8, p.H); y++ {
					p.P[y*p.W+bx+7] = float32(clamp(float64(p.P[y*p.W+bx+7]-7), 0, 255))
				}
			}
		}
	}
}

func writeTestPNG(t *testing.T, path string, p *plane) string {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, p.W, p.H))
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(clamp(p.at(x, y), 0, 255))})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func clampInt(v, low, high int) int {
	return max(low, min(high, v))
}
