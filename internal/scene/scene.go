// Package scene 生成带已知线性光真值的合成测试场景。
package scene

import (
	"math/rand"

	"itm/internal/img"
)

// Scene 是一张 HDR 线性光真值图及其命名区域（用于分区统计）。
type Scene struct {
	Name    string
	Image   *img.Image
	Regions map[string]img.Rect
}

const size = 256

// ExposureStaircase 生成 9 级曝光阶梯，真值为 2^(-5)...2^3，
// 覆盖暗部、中间调、可反演高光（>1 但未在 SDR 裁切）与裁切高光。
func ExposureStaircase() *Scene {
	m := img.New(size, size)
	regions := map[string]img.Rect{}
	bands := 9
	bw := size / bands
	for k := 0; k < bands; k++ {
		v := 1.0 // 2^(k-5)
		for e := 0; e < k-5; e++ {
			v *= 2
		}
		for e := 0; e < 5-k; e++ {
			v *= 0.5
		}
		x1 := (k + 1) * bw
		if k == bands-1 {
			x1 = size
		}
		rc := img.Rect{X0: k * bw, Y0: 0, X1: x1, Y1: size}
		m.FillRect(rc, v, v, v)
		regions["step"+itoa(k)] = rc
	}
	return &Scene{Name: "exposure_staircase", Image: m, Regions: regions}
}

// ControlledHighlights 生成含天空、人脸、漫反射白与小镜面高光的受控场景。
func ControlledHighlights() *Scene {
	m := img.New(size, size)
	m.FillRect(img.Rect{X0: 0, Y0: 0, X1: size, Y1: size}, 0.05, 0.05, 0.06)
	regions := map[string]img.Rect{}

	sky := img.Rect{X0: 0, Y0: 0, X1: size, Y1: size / 2}
	m.FillRect(sky, 0.55, 0.68, 0.90)
	regions["sky"] = sky

	face := img.Rect{X0: 32, Y0: 140, X1: 128, Y1: 220}
	m.FillRect(face, 0.45, 0.28, 0.20)
	regions["face"] = face

	diffuse := img.Rect{X0: 150, Y0: 140, X1: 210, Y1: 200}
	m.FillRect(diffuse, 0.90, 0.90, 0.88)
	regions["diffuse_white"] = diffuse

	spec := img.Rect{X0: 215, Y0: 60, X1: 245, Y1: 90}
	m.FillRect(spec, 6.0, 6.0, 5.5)
	regions["specular"] = spec

	dark := img.Rect{X0: 150, Y0: 215, X1: 210, Y1: 245}
	m.FillRect(dark, 0.04, 0.04, 0.05)
	regions["dark"] = dark
	return &Scene{Name: "controlled_highlights", Image: m, Regions: regions}
}

// HueChart 生成饱和原色块与肤色块，用于色相保持评估。
func HueChart() *Scene {
	m := img.New(size, size)
	regions := map[string]img.Rect{}
	type patch struct {
		name     string
		r, g, b  float64
		col, row int
	}
	patches := []patch{
		{"red", 0.70, 0.05, 0.05, 0, 0},
		{"green", 0.05, 0.60, 0.08, 1, 0},
		{"blue", 0.05, 0.10, 0.70, 2, 0},
		{"yellow", 0.75, 0.70, 0.08, 0, 1},
		{"cyan", 0.08, 0.65, 0.70, 1, 1},
		{"magenta", 0.60, 0.08, 0.55, 2, 1},
		{"skin", 0.45, 0.28, 0.20, 1, 2},
	}
	w, h := 70, 70
	for _, p := range patches {
		x0 := 20 + p.col*78
		y0 := 15 + p.row*78
		rc := img.Rect{X0: x0, Y0: y0, X1: x0 + w, Y1: y0 + h}
		m.FillRect(rc, p.r, p.g, p.b)
		regions[p.name] = rc
	}
	return &Scene{Name: "hue_chart", Image: m, Regions: regions}
}

// NoiseChart 生成带已知高斯噪声（线性域）的中间调块与高光块。
func NoiseChart() *Scene {
	m := img.New(size, size)
	rng := rand.New(rand.NewSource(20261127))

	mid := img.Rect{X0: 0, Y0: 0, X1: size, Y1: size / 2}
	fillNoise(m, mid, 0.18, 0.01, rng)

	high := img.Rect{X0: 0, Y0: size / 2, X1: size, Y1: size}
	fillNoise(m, high, 0.80, 0.02, rng)

	return &Scene{
		Name:    "noise_chart",
		Image:   m,
		Regions: map[string]img.Rect{"midtone": mid, "highlight": high},
	}
}

func fillNoise(m *img.Image, rc img.Rect, base, sigma float64, rng *rand.Rand) {
	for y := rc.Y0; y < rc.Y1; y++ {
		for x := rc.X0; x < rc.X1; x++ {
			n := rng.NormFloat64() * sigma
			v := base + n
			if v < 0 {
				v = 0
			}
			m.Set(x, y, v, v, v)
		}
	}
}

// All 返回全部测试场景。
func All() []*Scene {
	return []*Scene{
		ExposureStaircase(),
		ControlledHighlights(),
		HueChart(),
		NoiseChart(),
	}
}

func itoa(k int) string {
	if k == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for k > 0 {
		i--
		buf[i] = byte('0' + k%10)
		k /= 10
	}
	return string(buf[i:])
}
