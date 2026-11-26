package tmo

import "math"

// gaussianKernel 按 3*sigma 截断生成一维归一化核。
func gaussianKernel(sigma float64) []float64 {
	r := int(math.Ceil(3 * sigma))
	if r < 1 {
		r = 1
	}
	k := make([]float64, 2*r+1)
	sum := 0.0
	inv2s2 := 1.0 / (2 * sigma * sigma)
	for i := -r; i <= r; i++ {
		v := math.Exp(-float64(i*i) * inv2s2)
		k[i+r] = v
		sum += v
	}
	for i := range k {
		k[i] /= sum
	}
	return k
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// gaussianBlur 对灰度图做可分离高斯模糊，越界采用边缘复制。
func gaussianBlur(src []float64, w, h int, sigma float64) []float64 {
	k := gaussianKernel(sigma)
	r := len(k) / 2
	tmp := make([]float64, w*h)
	out := make([]float64, w*h)

	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			acc := 0.0
			for j := -r; j <= r; j++ {
				acc += k[j+r] * src[row+clamp(x+j, 0, w-1)]
			}
			tmp[row+x] = acc
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			acc := 0.0
			for j := -r; j <= r; j++ {
				yy := clamp(y+j, 0, h-1)
				acc += k[j+r] * tmp[yy*w+x]
			}
			out[y*w+x] = acc
		}
	}
	return out
}
