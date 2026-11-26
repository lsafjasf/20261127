// Package tmo 实现正向色调映射（HDR->SDR 相机模型）与三种逆色调映射策略。
package tmo

import (
	"math"

	"itm/internal/color"
	"itm/internal/img"
)

// Forward 是广义 Reinhard 色调映射曲线的标量形式：
//
//	T(x) = x*(1 + x/w^2) / (1 + x)，其中 n 控制肩部形状（n=1 为经典 Reinhard）。
//
// 输出裁剪到 [0,1]，对应 SDR 白。
func Forward(x, n, white float64) float64 {
	if x <= 0 {
		return 0
	}
	y := x * (1 + math.Pow(x/white, n)) / (1 + x)
	if y > 1 {
		return 1
	}
	return y
}

// Quantize8 在 sRGB 编码域做 8 位量化，再解码回线性光，
// 模拟真实 SDR 素材经历的 gamma 域量化误差。
func Quantize8(lin float64) float64 {
	enc := color.Encode(lin)
	q := math.Round(enc*255.0) / 255.0
	return color.Decode(q)
}

// CameraToSDR 用给定肩参数把一张线性光 HDR 图压成 SDR（线性光表示，已含量化）。
func CameraToSDR(hdr *img.Image, n, white float64) *img.Image {
	sdr := img.New(hdr.W, hdr.H)
	for i := range hdr.R {
		sdr.R[i] = Quantize8(Forward(hdr.R[i], n, white))
		sdr.G[i] = Quantize8(Forward(hdr.G[i], n, white))
		sdr.B[i] = Quantize8(Forward(hdr.B[i], n, white))
	}
	return sdr
}
