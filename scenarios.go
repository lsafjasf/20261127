package aperturemap

// Combination is a named canonical geometry construction.
type Combination struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Spec  Spec   `json:"spec"`
}

// CanonicalCombinations returns the four required fact combinations.
//
//  1. Effective pixels only: square pixels and no clap declaration.
//  2. Effective pixels + PAR: a 4:3 raster is widened to 16:9 by PAR.
//  3. Effective pixels + clap: coding padding is removed before display.
//  4. All four facts: fractional-capable clap, PAR and explicit DAR agree.
func CanonicalCombinations() []Combination {
	dar16x9 := MustRatio(16, 9)
	viewport := Size{Width: 1920, Height: 1080}

	return []Combination{
		{
			ID:    "effective-only-square",
			Title: "有效像素 + 方形像素，无 clean aperture",
			Spec: Spec{
				Name:            "effective-only-square",
				EffectivePixels: Size{Width: 3840, Height: 2160},
				Viewport:        &viewport,
				Fit:             FitExact,
			},
		},
		{
			ID:    "effective-par-anamorphic",
			Title: "有效像素 + PAR 恢复显示比例",
			Spec: Spec{
				Name:             "effective-par-anamorphic",
				EffectivePixels:  Size{Width: 1440, Height: 1080},
				PixelAspectRatio: MustRatio(4, 3),
				Viewport:         &viewport,
				Fit:              FitExact,
			},
		},
		{
			ID:    "effective-clap-padding",
			Title: "有效像素 + clean aperture 去除编码填充",
			Spec: Spec{
				Name:            "effective-clap-padding",
				EffectivePixels: Size{Width: 1920, Height: 1088},
				CleanAperture: &CleanAperture{
					Width:          MustRatio(1920, 1),
					Height:         MustRatio(1080, 1),
					VerticalOffset: MustRatio(-4, 1),
				},
				Viewport: &viewport,
				Fit:      FitExact,
			},
		},
		{
			ID:    "all-declarations-compound",
			Title: "有效像素 + PAR + DAR + clean aperture 联合校验",
			Spec: Spec{
				Name:               "all-declarations-compound",
				EffectivePixels:    Size{Width: 1440, Height: 1080},
				PixelAspectRatio:   MustRatio(4, 3),
				DisplayAspectRatio: &dar16x9,
				CleanAperture: &CleanAperture{
					Width:  MustRatio(1368, 1),
					Height: MustRatio(1026, 1),
				},
				Viewport: &viewport,
				Fit:      FitExact,
			},
		},
	}
}

// FourCombinations returns the required four-case geometry matrix.
func FourCombinations() []Combination {
	return CanonicalCombinations()
}

// RejectionExamples returns declarations that must not be resolved.
func RejectionExamples() []Combination {
	dar4x3 := MustRatio(4, 3)
	return []Combination{
		{
			ID:    "dar-conflicts-with-par-and-pixels",
			Title: "显式 DAR 与像素/PAR 几何冲突",
			Spec: Spec{
				EffectivePixels:    Size{Width: 1440, Height: 1080},
				PixelAspectRatio:   MustRatio(4, 3),
				DisplayAspectRatio: &dar4x3,
			},
		},
		{
			ID:    "clap-outside-effective-grid",
			Title: "clean aperture 超出有效像素边界",
			Spec: Spec{
				EffectivePixels: Size{Width: 1920, Height: 1080},
				CleanAperture: &CleanAperture{
					Width:            MustRatio(1920, 1),
					Height:           MustRatio(1080, 1),
					HorizontalOffset: MustRatio(2, 1),
				},
			},
		},
	}
}

// PlayerMismatchExamples shows the same 16:9 source in a wider 2:1 surface.
// Stretch widens pixels, cover re-crops vertically, and exact rejects.
func PlayerMismatchExamples() []Combination {
	wide := Size{Width: 2000, Height: 1000}
	base := Spec{
		EffectivePixels: Size{Width: 1920, Height: 1080},
		Viewport:        &wide,
	}

	stretch := base
	stretch.Name = "wide-viewport-stretch"
	stretch.Fit = FitStretch

	cover := base
	cover.Name = "wide-viewport-cover"
	cover.Fit = FitCover

	exact := base
	exact.Name = "wide-viewport-exact-rejected"
	exact.Fit = FitExact

	return []Combination{
		{
			ID:    "wide-viewport-stretch",
			Title: "播放器强制拉伸到更宽视口",
			Spec:  stretch,
		},
		{
			ID:    "wide-viewport-cover",
			Title: "播放器 cover 再次裁切画面",
			Spec:  cover,
		},
		{
			ID:    "wide-viewport-exact-rejected",
			Title: "要求不变形且不二次裁切时拒绝",
			Spec:  exact,
		},
	}
}
