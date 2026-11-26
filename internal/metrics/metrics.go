// Package metrics 在已知真值区域上比较逆色调映射结果。
package metrics

import (
	"math"

	"itm/internal/color"
	"itm/internal/img"
)

// LuminanceRMSE 在区域内计算亮度（线性光）RMSE。
func LuminanceRMSE(gt, est *img.Image, rc img.Rect) float64 {
	sse, n := 0.0, 0
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			i := y*gt.W + x
			lg := color.Luminance(gt.R[i], gt.G[i], gt.B[i])
			le := color.Luminance(est.R[i], est.G[i], est.B[i])
			sse += (lg - le) * (lg - le)
			n++
		}
	}
	return math.Sqrt(sse / float64(n))
}

// LogRMSE 在 log10 亮度上计算 RMSE，对暗部与宽动态范围更敏感。
func LogRMSE(gt, est *img.Image, rc img.Rect) float64 {
	sse, n := 0.0, 0
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			i := y*gt.W + x
			lg := math.Log10(color.Luminance(gt.R[i], gt.G[i], gt.B[i]) + 1e-6)
			le := math.Log10(color.Luminance(est.R[i], est.G[i], est.B[i]) + 1e-6)
			sse += (lg - le) * (lg - le)
			n++
		}
	}
	return math.Sqrt(sse / float64(n))
}

// HighlightError 只统计真值亮度 >= threshold 的像素，返回亮度相对误差均值。
func HighlightError(gt, est *img.Image, rc img.Rect, threshold float64) (float64, int) {
	sum, n := 0.0, 0
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			i := y*gt.W + x
			lg := color.Luminance(gt.R[i], gt.G[i], gt.B[i])
			if lg < threshold {
				continue
			}
			le := color.Luminance(est.R[i], est.G[i], est.B[i])
			sum += math.Abs(le-lg) / lg
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

// HueError 统计区域内彩色像素的平均圆周色相误差（度）与平均色度比变化。
func HueError(gt, est *img.Image, rc img.Rect) (deg float64, chromaRatio float64, n int) {
	hs, cs := 0.0, 0.0
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			i := y*gt.W + x
			hg, okG := color.HueDeg(gt.R[i], gt.G[i], gt.B[i])
			he, okE := color.HueDeg(est.R[i], est.G[i], est.B[i])
			if !okG || !okE {
				continue
			}
			hs += color.HueDelta(hg, he)
			cg := color.Chroma(gt.R[i], gt.G[i], gt.B[i])
			ce := color.Chroma(est.R[i], est.G[i], est.B[i])
			if cg > 1e-9 {
				cs += ce / cg
			}
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	return hs / float64(n), cs / float64(n), n
}

// LumaStd 返回区域内亮度标准差，用于衡量噪声水平。
func LumaStd(m *img.Image, rc img.Rect) (mean, std float64) {
	n := 0
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			i := y*m.W + x
			mean += color.Luminance(m.R[i], m.G[i], m.B[i])
			n++
		}
	}
	mean /= float64(n)
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			i := y*m.W + x
			l := color.Luminance(m.R[i], m.G[i], m.B[i])
			std += (l - mean) * (l - mean)
		}
	}
	return mean, math.Sqrt(std / float64(n))
}

// NoiseAmplification 返回估计图相对真值图的噪声标准差倍数。
func NoiseAmplification(gt, est *img.Image, rc img.Rect) (gtStd, estStd, ratio float64) {
	_, gtStd = LumaStd(gt, rc)
	_, estStd = LumaStd(est, rc)
	if gtStd < 1e-12 {
		return gtStd, estStd, 0
	}
	return gtStd, estStd, estStd / gtStd
}
