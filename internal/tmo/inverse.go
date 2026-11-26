package tmo

import (
	"math"

	"itm/internal/color"
	"itm/internal/img"
)

// LinearExpand 策略一：朴素线性扩展 x = gain*y。
// 这正是“直接按 gamma 反向”式做法的线性域等价物：暗部噪声与中间调人脸
// 被同比例放大，SDR 白被直接映射成固定的 HDR 峰值。
func LinearExpand(sdr *img.Image, gain float64) *img.Image {
	out := img.New(sdr.W, sdr.H)
	for i := range sdr.R {
		out.R[i] = gain * sdr.R[i]
		out.G[i] = gain * sdr.G[i]
		out.B[i] = gain * sdr.B[i]
	}
	return out
}

// invertForward 在单调的 Forward 曲线上用二分法求 x 使 Forward(x)=y。
// y==1（SDR 裁切）无法唯一反演，返回 upper 作为保守外推上界。
func invertForward(y, n, white, upper float64) float64 {
	if y <= 0 {
		return 0
	}
	if y >= 1 {
		return upper
	}
	lo, hi := 0.0, upper
	for k := 0; k < 60; k++ {
		mid := (lo + hi) * 0.5
		if Forward(mid, n, white) < y {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) * 0.5
}

// SoftShoulder 策略二：软肩重建。逐通道反演已知正向曲线，
// 中间调近似 1:1 通过（人脸不被整体抬亮），高光沿软肩外推到 upper。
func SoftShoulder(sdr *img.Image, n, white, upper float64) *img.Image {
	out := img.New(sdr.W, sdr.H)
	for i := range sdr.R {
		out.R[i] = invertForward(sdr.R[i], n, white, upper)
		out.G[i] = invertForward(sdr.G[i], n, white, upper)
		out.B[i] = invertForward(sdr.B[i], n, white, upper)
	}
	return out
}

func smoothstep(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}

// LocalProtect 策略三：局部内容保护。
//
// 以软肩重建给出全局增益 g_base(y)=F^{-1}(y)/y，再用大尺度高斯模糊得到
// 局部平均亮度 L 与局部对比度 C=局部标准差/局部均值。
// 亮且平坦（C 低）的漫反射内容（天空、人脸）把增益向 floor 收缩，
// 避免整体异常发亮；高对比度局部亮点（镜面高光）保留软肩外推增益。
// 三个通道共用同一亮度增益，因此色相在数学上被严格保持。
//
// low/high 为 SDR 局部平均亮度的保护门限，floor 为亮区保留的增益比例，
// sigma 为大尺度局部分析半径。
func LocalProtect(sdr *img.Image, n, white, upper, sigma float64, low, high, floor float64) *img.Image {
	base := SoftShoulder(sdr, n, white, upper)

	luma := make([]float64, sdr.W*sdr.H)
	for i := range sdr.R {
		luma[i] = color.Luminance(sdr.R[i], sdr.G[i], sdr.B[i])
	}
	localLuma := gaussianBlur(luma, sdr.W, sdr.H, sigma)
	sq := make([]float64, len(luma))
	for i, v := range luma {
		sq[i] = v * v
	}
	localSq := gaussianBlur(sq, sdr.W, sdr.H, sigma)

	out := img.New(sdr.W, sdr.H)
	for i := range sdr.R {
		var gBase float64
		if luma[i] > 1e-9 {
			gBase = color.Luminance(base.R[i], base.G[i], base.B[i]) / luma[i]
		} else {
			gBase = 1
		}
		variance := localSq[i] - localLuma[i]*localLuma[i]
		if variance < 0 {
			variance = 0
		}
		contrast := math.Sqrt(variance) / (localLuma[i] + 1e-6)
		// bright 随局部平均亮度增大：暗邻域（含暗背景上的镜面点）不保护。
		bright := smoothstep((localLuma[i] - low) / (high - low))
		// flat 随局部对比度降低而增大；对比度超过 0.35 视为含局部高光，解除保护。
		flat := 1 - smoothstep(contrast/0.35)
		protect := bright * flat
		gain := gBase*(1-protect) + math.Max(1, gBase*floor)*protect
		out.R[i] = gain * sdr.R[i]
		out.G[i] = gain * sdr.G[i]
		out.B[i] = gain * sdr.B[i]
	}
	return out
}
