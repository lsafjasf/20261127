// Package color 提供 sRGB 传输函数与基础颜色空间换算，全部使用 float64 线性光 RGB。
package color

import "math"

// Decode 把 sRGB 编码值 [0,1] 转为线性光值。
func Decode(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// Encode 把线性光值编码为 sRGB 值，结果裁剪到 [0,1]。
func Encode(c float64) float64 {
	if c <= 0.0 {
		return 0
	}
	if c >= 1.0 {
		return 1
	}
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1.0/2.4) - 0.055
}

// Luminance 返回 Rec.709 相对亮度。
func Luminance(r, g, b float64) float64 {
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// HueDeg 返回色相角（度，0-360）；近灰像素返回 -1 与 ok=false。
func HueDeg(r, g, b float64) (float64, bool) {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	delta := mx - mn
	if delta < 1e-12 {
		return -1, false
	}
	var h float64
	switch mx {
	case r:
		h = math.Mod((g-b)/delta, 6)
	case g:
		h = (b-r)/delta + 2
	default:
		h = (r-g)/delta + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, true
}

// HueDelta 返回两个色相角之间的最短圆周距离（度）。
func HueDelta(h1, h2 float64) float64 {
	d := math.Abs(h1 - h2)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// Chroma 返回简单色度（max-min），用于衡量饱和度变化。
func Chroma(r, g, b float64) float64 {
	return math.Max(r, math.Max(g, b)) - math.Min(r, math.Min(g, b))
}
