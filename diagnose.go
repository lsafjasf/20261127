package aperturemap

// Diagnosis summarizes whether the final mapping stretches or re-crops content.
type Diagnosis struct {
	// PhysicalStretch is (viewport x-scale) / (PAR-corrected y-scale).
	// 1 means geometrically uniform after honoring PAR.
	PhysicalStretch      Ratio `json:"physical_stretch_factor"`
	UniformPhysicalScale bool  `json:"uniform_physical_scale"`

	// PlayerCropWidthFraction/HeightFraction are fractions of the resolved
	// clean source rectangle removed by a cover viewport.
	PlayerCropped      bool  `json:"player_cropped"`
	CropWidthFraction  Ratio `json:"crop_width_fraction"`
	CropHeightFraction Ratio `json:"crop_height_fraction"`

	// HorizontalBarFraction/VerticalBarFraction are total letterbox fractions.
	Letterboxed           bool  `json:"letterboxed"`
	HorizontalBarFraction Ratio `json:"horizontal_bar_fraction"`
	VerticalBarFraction   Ratio `json:"vertical_bar_fraction"`
}

func diagnose(result *Resolved, spec Spec) Diagnosis {
	one := MustRatio(1, 1)
	stretch := result.Transform.ScaleX.Div(result.PixelAspectRatio.Mul(result.Transform.ScaleY))
	diagnosis := Diagnosis{
		PhysicalStretch:       stretch,
		UniformPhysicalScale:  stretch.Equal(one),
		CropWidthFraction:     Ratio{N: 0, D: 1},
		CropHeightFraction:    Ratio{N: 0, D: 1},
		HorizontalBarFraction: Ratio{N: 0, D: 1},
		VerticalBarFraction:   Ratio{N: 0, D: 1},
	}

	if result.VisibleSourceRect != nil {
		visible := *result.VisibleSourceRect
		sourceW := result.SourceRect.Width()
		sourceH := result.SourceRect.Height()
		diagnosis.CropWidthFraction = one.Sub(visible.Width().Div(sourceW))
		diagnosis.CropHeightFraction = one.Sub(visible.Height().Div(sourceH))
		diagnosis.PlayerCropped = diagnosis.CropWidthFraction.N > 0 || diagnosis.CropHeightFraction.N > 0
	}

	if spec.Viewport != nil && result.Fit == FitContain {
		vpW := MustRatio(int64(spec.Viewport.Width), 1)
		vpH := MustRatio(int64(spec.Viewport.Height), 1)
		targetW := result.TargetRect.Width()
		targetH := result.TargetRect.Height()
		diagnosis.HorizontalBarFraction = vpW.Sub(targetW).Div(vpW)
		diagnosis.VerticalBarFraction = vpH.Sub(targetH).Div(vpH)
		diagnosis.Letterboxed = diagnosis.HorizontalBarFraction.N > 0 || diagnosis.VerticalBarFraction.N > 0
	}

	return diagnosis
}
