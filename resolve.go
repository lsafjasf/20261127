package aperturemap

import "fmt"

// Spec is the set of facts that must remain geometrically consistent.
type Spec struct {
	Name string `json:"name,omitempty"`

	// EffectivePixels is the integer camera/active raster before clap cropping.
	EffectivePixels Size `json:"effective_pixels"`

	// PixelAspectRatio is hSpacing/vSpacing. The zero value means square 1:1.
	PixelAspectRatio Ratio `json:"pixel_aspect_ratio,omitempty"`

	// DisplayAspectRatio is an optional independently declared final DAR.
	DisplayAspectRatio *Ratio `json:"display_aspect_ratio,omitempty"`

	// CleanAperture is the optional ISO BMFF/QuickTime clap declaration.
	CleanAperture *CleanAperture `json:"clean_aperture,omitempty"`

	// Viewport is the player surface in physical/logical pixels.
	Viewport *Size `json:"viewport,omitempty"`

	// Fit controls player behavior. Empty defaults to FitExact.
	Fit FitMode `json:"fit,omitempty"`
}

// CleanAperture is the exact rational clap declaration.
type CleanAperture struct {
	Width            Ratio `json:"width"`
	Height           Ratio `json:"height"`
	HorizontalOffset Ratio `json:"horizontal_offset,omitempty"`
	VerticalOffset   Ratio `json:"vertical_offset,omitempty"`
}

// IndexRange describes integer pixel centers enclosed by an exact rectangle.
type IndexRange struct {
	First int   `json:"first"`
	Last  int   `json:"last"`
	Count int64 `json:"count"`
}

// ResolvedAperture contains exact clap coordinates and integer sampling hints.
type ResolvedAperture struct {
	Declaration        CleanAperture `json:"declaration"`
	Center             Point         `json:"center"`
	ExactRect          Rect          `json:"exact_rect"`
	PixelX             *IndexRange   `json:"pixel_x,omitempty"`
	PixelY             *IndexRange   `json:"pixel_y,omitempty"`
	CleanPixelAspect   Ratio         `json:"clean_pixel_aspect"`
	CleanDisplayAspect Ratio         `json:"clean_display_aspect"`
}

// Mapping is one coordinate-stage transformation.
type Mapping struct {
	Name       string `json:"name"`
	From       string `json:"from"`
	To         string `json:"to"`
	SourceRect Rect   `json:"source_rect"`
	TargetRect Rect   `json:"target_rect"`
	Transform  Affine `json:"transform"`
}

// Resolved is the validated geometry and final mapping.
type Resolved struct {
	Name string `json:"name,omitempty"`

	EffectivePixels           Size  `json:"effective_pixels"`
	EffectivePixelAspect      Ratio `json:"effective_pixel_aspect"`
	SourcePixelAspect         Ratio `json:"source_pixel_aspect"`
	PixelAspectRatio          Ratio `json:"pixel_aspect_ratio"`
	FullApertureDisplayAspect Ratio `json:"full_aperture_display_aspect"`

	CleanAperture *ResolvedAperture `json:"clean_aperture,omitempty"`

	DisplayAspectRatio      Ratio  `json:"display_aspect_ratio"`
	DisplayAspectSource     string `json:"display_aspect_source"`
	DeclaredDisplayAccepted bool   `json:"declared_display_accepted"`

	Viewport *Size   `json:"viewport,omitempty"`
	Fit      FitMode `json:"fit"`

	SourceRect Rect `json:"source_rect_in_effective_pixel_coordinates"`
	TargetRect Rect `json:"target_rect_in_viewport_coordinates"`

	// Transform maps SourceRect to TargetRect. For cover, TargetRect may overflow.
	Transform Affine `json:"transform"`

	// VisibleSourceRect is the source region remaining after viewport clipping.
	VisibleSourceRect *Rect `json:"visible_source_rect,omitempty"`

	Diagnosis Diagnosis `json:"diagnosis"`

	Mappings []Mapping `json:"mappings"`
}

// Resolve validates all declarations and constructs the final geometry mapping.
func Resolve(spec Spec) (*Resolved, error) {
	if spec.EffectivePixels.Width <= 0 || spec.EffectivePixels.Height <= 0 {
		return nil, fmt.Errorf("%w: effective pixel dimensions must be positive", ErrInvalidGeometry)
	}

	par := spec.PixelAspectRatio
	if par.N == 0 && par.D == 0 {
		par = MustRatio(1, 1)
	}
	if !par.Positive() {
		return nil, fmt.Errorf("%w: pixel aspect ratio must be positive", ErrInvalidGeometry)
	}

	fullRect := PixelGridRect(spec.EffectivePixels.Width, spec.EffectivePixels.Height)
	effectivePAR := MustRatio(int64(spec.EffectivePixels.Width), int64(spec.EffectivePixels.Height))
	fullDAR := effectivePAR.Mul(par)

	result := &Resolved{
		Name:                      spec.Name,
		EffectivePixels:           spec.EffectivePixels,
		EffectivePixelAspect:      effectivePAR,
		SourcePixelAspect:         effectivePAR,
		PixelAspectRatio:          par,
		FullApertureDisplayAspect: fullDAR,
		Fit:                       spec.Fit,
		Viewport:                  spec.Viewport,
		SourceRect:                fullRect,
		DisplayAspectSource:       "derived-from-effective-pixels-and-par",
	}

	if spec.CleanAperture != nil {
		clean, err := resolveCleanAperture(spec, fullRect, par)
		if err != nil {
			return nil, err
		}
		result.CleanAperture = clean
		result.SourceRect = clean.ExactRect
		result.DisplayAspectRatio = clean.CleanDisplayAspect
		result.DisplayAspectSource = "derived-from-clean-aperture-and-par"
	} else {
		result.DisplayAspectRatio = fullDAR
	}

	if spec.DisplayAspectRatio != nil {
		declared := *spec.DisplayAspectRatio
		if !declared.Positive() {
			return nil, fmt.Errorf("%w: declared display aspect ratio must be positive", ErrInvalidGeometry)
		}
		if !declared.Equal(result.DisplayAspectRatio) {
			return nil, fmt.Errorf("%w: declared DAR %s != geometric DAR %s (effective=%s, PAR=%s, clean=%t)",
				ErrInconsistentRatio, declared, result.DisplayAspectRatio, effectivePAR, par, spec.CleanAperture != nil)
		}
		result.DeclaredDisplayAccepted = true
		result.DisplayAspectSource = "declared-and-verified"
	}

	if err := resolvePresentation(spec, result); err != nil {
		return nil, err
	}

	result.Diagnosis = diagnose(result, spec)
	result.Mappings = buildMappings(result)
	return result, nil
}

func resolveCleanAperture(spec Spec, fullRect Rect, par Ratio) (*ResolvedAperture, error) {
	decl := *spec.CleanAperture
	if !decl.Width.Positive() || !decl.Height.Positive() {
		return nil, fmt.Errorf("%w: clean aperture width/height must be positive", ErrInvalidGeometry)
	}

	// ISO/IEC 14496-12: pc = offset + (fullSize - 1)/2.
	centerX := MustRatio(int64(spec.EffectivePixels.Width)-1, 2).Add(decl.HorizontalOffset)
	centerY := MustRatio(int64(spec.EffectivePixels.Height)-1, 2).Add(decl.VerticalOffset)

	halfW := decl.Width.Mul(MustRatio(1, 2))
	halfH := decl.Height.Mul(MustRatio(1, 2))
	exact := NewRect(centerX.Sub(halfW), centerY.Sub(halfH), centerX.Add(halfW), centerY.Add(halfH))

	if !fullRect.Contains(exact) {
		return nil, fmt.Errorf("%w: clap rect %s is not contained by %s",
			ErrApertureOutOfBounds, rectString(exact), rectString(fullRect))
	}

	pixelX := enclosedIndices(exact.Min.X, exact.Max.X)
	pixelY := enclosedIndices(exact.Min.Y, exact.Max.Y)

	pixelAspect := decl.Width.Div(decl.Height)
	displayAspect := pixelAspect.Mul(par)

	return &ResolvedAperture{
		Declaration:        decl,
		Center:             Point{X: centerX, Y: centerY},
		ExactRect:          exact,
		PixelX:             pixelX,
		PixelY:             pixelY,
		CleanPixelAspect:   pixelAspect,
		CleanDisplayAspect: displayAspect,
	}, nil
}

func enclosedIndices(min, max Ratio) *IndexRange {
	first := ceilRat(min)
	last := floorRat(max)
	if first > last {
		return nil
	}
	return &IndexRange{First: first, Last: last, Count: int64(last - first + 1)}
}

func resolvePresentation(spec Spec, result *Resolved) error {
	mode := spec.Fit
	if mode == "" {
		mode = FitExact
	}
	result.Fit = mode
	switch mode {
	case FitExact, FitStretch, FitContain, FitCover:
	default:
		return fmt.Errorf("%w: unknown fit mode %q", ErrInvalidGeometry, mode)
	}

	normalized := NewRect(MustRatio(0, 1), MustRatio(0, 1), result.DisplayAspectRatio, MustRatio(1, 1))

	if spec.Viewport == nil {
		target := normalized
		result.TargetRect = target
		transform, err := RectMap(result.SourceRect, target)
		if err != nil {
			return err
		}
		result.Transform = transform
		return nil
	}

	vp := *spec.Viewport
	if vp.Width <= 0 || vp.Height <= 0 {
		return fmt.Errorf("%w: viewport dimensions must be positive", ErrInvalidGeometry)
	}
	viewportRect := NewRect(Ratio{}, Ratio{}, MustRatio(int64(vp.Width), 1), MustRatio(int64(vp.Height), 1))
	viewportAR := MustRatio(int64(vp.Width), int64(vp.Height))
	dar := result.DisplayAspectRatio

	switch mode {
	case FitExact:
		if !viewportAR.Equal(dar) {
			return fmt.Errorf("%w: exact fit requires viewport AR %s to equal clean DAR %s; choose contain/cover/stretch or correct metadata",
				ErrPresentationImpossible, viewportAR, dar)
		}
		result.TargetRect = viewportRect

	case FitStretch:
		result.TargetRect = viewportRect

	case FitContain, FitCover:
		scale := viewportRect.Width().Div(dar)
		heightScale := viewportRect.Height()
		if mode == FitContain {
			if scale.Cmp(heightScale) > 0 {
				scale = heightScale
			}
		} else {
			if scale.Cmp(heightScale) < 0 {
				scale = heightScale
			}
		}
		targetW := dar.Mul(scale)
		targetH := scale
		offsetX := viewportRect.Width().Sub(targetW).Mul(MustRatio(1, 2))
		offsetY := viewportRect.Height().Sub(targetH).Mul(MustRatio(1, 2))
		result.TargetRect = NewRect(offsetX, offsetY, offsetX.Add(targetW), offsetY.Add(targetH))

	default:
		return fmt.Errorf("%w: unknown fit mode %q", ErrInvalidGeometry, mode)
	}

	transform, err := RectMap(result.SourceRect, result.TargetRect)
	if err != nil {
		return err
	}
	result.Transform = transform

	if mode == FitCover {
		visible := transform.Inverse().MapRect(viewportRect)
		clipped := visible.Intersect(result.SourceRect)
		result.VisibleSourceRect = &clipped
	}
	return nil
}

func buildMappings(result *Resolved) []Mapping {
	sourceLocal := NewRect(Ratio{}, Ratio{}, result.SourceRect.Width(), result.SourceRect.Height())
	cropMap, _ := RectMap(result.SourceRect, sourceLocal)

	normalized := NewRect(Ratio{}, Ratio{}, result.DisplayAspectRatio, MustRatio(1, 1))
	normalizeMap, _ := RectMap(sourceLocal, normalized)

	placementMap, _ := RectMap(normalized, result.TargetRect)

	return []Mapping{
		{
			Name:       "aperture-crop",
			From:       "effective-pixel outer-edge coordinates",
			To:         "clean-aperture local outer-edge coordinates",
			SourceRect: result.SourceRect,
			TargetRect: sourceLocal,
			Transform:  cropMap,
		},
		{
			Name:       "par-and-normalize",
			From:       "clean-aperture counted pixels",
			To:         "physical aspect coordinates (height normalized to 1)",
			SourceRect: sourceLocal,
			TargetRect: normalized,
			Transform:  normalizeMap,
		},
		{
			Name:       "viewport-placement",
			From:       "normalized display aperture",
			To:         "player viewport coordinates",
			SourceRect: normalized,
			TargetRect: result.TargetRect,
			Transform:  placementMap,
		},
	}
}

func rectString(r Rect) string {
	return fmt.Sprintf("x=[%s,%s], y=[%s,%s]", r.Min.X, r.Max.X, r.Min.Y, r.Max.Y)
}

func floorRat(r Ratio) int {
	v := r.N / r.D
	rem := r.N % r.D
	if rem != 0 && ((r.N < 0) != (r.D < 0)) {
		v--
	}
	return int(v)
}

func ceilRat(r Ratio) int {
	v := r.N / r.D
	rem := r.N % r.D
	if rem != 0 && ((r.N > 0) == (r.D > 0)) {
		v++
	}
	return int(v)
}
