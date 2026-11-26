// Package experiment 编排 SDR 合成、三种逆色调映射策略与指标汇总。
package experiment

import (
	"fmt"

	"itm/internal/img"
	"itm/internal/metrics"
	"itm/internal/scene"
	"itm/internal/tmo"
)

// Params 汇总正向相机与三种逆策略的参数。
type Params struct {
	ShoulderN    float64 // 正向广义 Reinhard 肩指数
	White        float64 // 正向白参考
	Upper        float64 // 逆映射外推上界（HDR 峰值目标）
	LinearGain   float64 // 策略一固定增益
	LocalSigma   float64 // 策略三大尺度模糊半径
	LocalLow     float64 // 局部亮度保护下限
	LocalHigh    float64 // 局部亮度保护上限
	LocalFloor   float64 // 亮区保留的最小增益比例
	HighlightThr float64 // 高光判定阈值
}

// DefaultParams 返回实验默认参数。
func DefaultParams() Params {
	return Params{
		ShoulderN:    1.2,
		White:        1.0,
		Upper:        8.0,
		LinearGain:   4.0,
		LocalSigma:   12,
		LocalLow:     0.20,
		LocalHigh:    0.60,
		LocalFloor:   0.35,
		HighlightThr: 1.0,
	}
}

// Strategy 是一个命名的逆色调映射算子。
type Strategy struct {
	Name string
	Run  func(sdr *img.Image, p Params) *img.Image
}

// Strategies 返回三种策略，顺序即报告顺序。
func Strategies() []Strategy {
	return []Strategy{
		{
			Name: "linear_expand",
			Run: func(sdr *img.Image, p Params) *img.Image {
				return tmo.LinearExpand(sdr, p.LinearGain)
			},
		},
		{
			Name: "soft_shoulder",
			Run: func(sdr *img.Image, p Params) *img.Image {
				return tmo.SoftShoulder(sdr, p.ShoulderN, p.White, p.Upper)
			},
		},
		{
			Name: "local_protect",
			Run: func(sdr *img.Image, p Params) *img.Image {
				return tmo.LocalProtect(sdr, p.ShoulderN, p.White, p.Upper,
					p.LocalSigma, p.LocalLow, p.LocalHigh, p.LocalFloor)
			},
		},
	}
}

// RegionMetrics 是单个场景、单个区域、单个策略的全部指标。
type RegionMetrics struct {
	Scene, Region, Strategy string
	RMSE, LogRMSE           float64
	HighlightRelErr         float64
	HighlightPixels         int
	HueDeg                  float64
	ChromaRatio             float64
	ColorPixels             int
	GTStd, EstStd, NoiseR   float64
}

// Run 对全部场景执行完整实验并返回逐区域指标。
func Run(p Params) []RegionMetrics {
	var out []RegionMetrics
	for _, sc := range scene.All() {
		sdr := tmo.CameraToSDR(sc.Image, p.ShoulderN, p.White)
		for _, st := range Strategies() {
			est := st.Run(sdr, p)
			for regionName, rc := range sc.Regions {
				hl, hlN := metrics.HighlightError(sc.Image, est, rc, p.HighlightThr)
				hue, chroma, colorN := metrics.HueError(sc.Image, est, rc)
				gtStd, estStd, noiseR := metrics.NoiseAmplification(sc.Image, est, rc)
				out = append(out, RegionMetrics{
					Scene: sc.Name, Region: regionName, Strategy: st.Name,
					RMSE:            metrics.LuminanceRMSE(sc.Image, est, rc),
					LogRMSE:         metrics.LogRMSE(sc.Image, est, rc),
					HighlightRelErr: hl, HighlightPixels: hlN,
					HueDeg: hue, ChromaRatio: chroma, ColorPixels: colorN,
					GTStd: gtStd, EstStd: estStd, NoiseR: noiseR,
				})
			}
		}
	}
	return out
}

// Aggregate 按策略聚合（均值），用于总览表。
type Aggregate struct {
	Strategy               string
	MeanRMSE, MeanLogRMSE  float64
	MeanHueDeg, MeanNoiseR float64
}

// AggregateByStrategy 汇总整体均值。
func AggregateByStrategy(rows []RegionMetrics) []Aggregate {
	var aggs []Aggregate
	for _, st := range Strategies() {
		var a Aggregate
		a.Strategy = st.Name
		nRMSE, nHue, nNoise := 0, 0, 0
		for _, r := range rows {
			if r.Strategy != st.Name {
				continue
			}
			a.MeanRMSE += r.RMSE
			a.MeanLogRMSE += r.LogRMSE
			nRMSE++
			if r.ColorPixels > 0 {
				a.MeanHueDeg += r.HueDeg
				nHue++
			}
			if r.GTStd > 1e-9 {
				a.MeanNoiseR += r.NoiseR
				nNoise++
			}
		}
		a.MeanRMSE /= float64(nRMSE)
		a.MeanLogRMSE /= float64(nRMSE)
		if nHue > 0 {
			a.MeanHueDeg /= float64(nHue)
		}
		if nNoise > 0 {
			a.MeanNoiseR /= float64(nNoise)
		}
		aggs = append(aggs, a)
	}
	return aggs
}

func (a Aggregate) String() string {
	return fmt.Sprintf("%-15s RMSE=%.4f logRMSE=%.4f hueErr=%.3f° noiseR=%.3f",
		a.Strategy, a.MeanRMSE, a.MeanLogRMSE, a.MeanHueDeg, a.MeanNoiseR)
}
