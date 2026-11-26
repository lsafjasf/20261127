// Package viz 把线性光 HDR 图像编码为可查看的 PNG。
package viz

import (
	"image"
	stdcolor "image/color"
	"image/png"
	"math"
	"os"

	lumcolor "itm/internal/color"
	"itm/internal/img"
)

// displayCurve 是仅用于可视化的显示映射（Reinhard 归一化），
// 不参与任何指标计算；peak 为本次拼图中的全局峰值亮度。
func displayCurve(x, peak float64) float64 {
	norm := x / peak
	return norm / (1 + norm)
}

// SavePNG 以给定峰值亮度归一化后写出 8 位 sRGB PNG。
func SavePNG(path string, m *img.Image, peak float64) error {
	dst := image.NewNRGBA(image.Rect(0, 0, m.W, m.H))
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			i := y*m.W + x
			r := lumcolor.Encode(displayCurve(m.R[i], peak))
			g := lumcolor.Encode(displayCurve(m.G[i], peak))
			b := lumcolor.Encode(displayCurve(m.B[i], peak))
			dst.SetNRGBA(x, y, stdcolor.NRGBA{
				R: clamp8(r),
				G: clamp8(g),
				B: clamp8(b),
				A: 255,
			})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, dst)
}

func clamp8(v float64) uint8 {
	v = math.Round(v * 255)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
