package iccproof

import "context"

// EngineOutput is the backend result before report classification.
//
// MediaValues are normalized to nominal channel range 0..1; MediaRawValues
// preserve backend units (for example LCMS percentage CMYK). MediaEncoded is
// the quantized encoding in 0..1 and MediaCodes contains integer code values.
type EngineOutput struct {
	SourceLab        *Lab
	SourceGamutAlarm *bool

	MediaColorSpace string
	MediaValues     []float64
	MediaRawValues  []float64
	GamutValues     []float64
	MediaEncoded    []float64
	MediaCodes      []int
	MediaLab        *Lab

	DisplayColorSpace string
	DisplayValues     []float64
	DisplayCodes      []int
	DisplayLab        *Lab
	MediaGamutAlarm   bool
}

// Engine is the color-management backend. LCMS is the production ICC engine;
// the deterministic reference engine is for explicit descriptors and tests.
type Engine interface {
	ProofSample(ctx context.Context, intent Intent, sample Sample, normalizedRGB [3]float64) (EngineOutput, error)
	Close() error
}

// DisplayAttacher is implemented by engines that can append a display-referred
// simulation after media encoding.
type DisplayAttacher interface {
	AttachDisplay(display *LoadedProfile) error
}

type engineFactory func(source, media *LoadedProfile, cfg ProofConfig) (Engine, error)

var testEngineFactory engineFactory

// NewEngine selects an injected engine, explicit descriptor math, or the ICC
// backend. It never creates an implicit sRGB profile.
func NewEngine(source, media *LoadedProfile, cfg ProofConfig) (Engine, error) {
	if testEngineFactory != nil {
		return testEngineFactory(source, media, cfg)
	}
	if source.Ref.Descriptor != nil && media.Ref.Descriptor != nil {
		return NewReferenceEngine(source.Ref.Descriptor, media.Ref.Descriptor, cfg)
	}
	return newICCEngine(source, media, cfg)
}
