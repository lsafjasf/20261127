package iccproof

import (
	"context"
	"fmt"
	"math"
)

// ReferenceEngine is a deterministic matrix/shaper CMM for explicitly
// supplied RGB profiles. It exists for tests, examples, and cgo-free builds;
// it is never instantiated as an implicit sRGB default.
type ReferenceEngine struct {
	source     *ColorProfile
	media      *ColorProfile
	display    *ColorProfile
	cfg        ProofConfig
	srcM       [3][3]float64
	dstInv     [3][3]float64
	displayInv [3][3]float64
}

func NewReferenceEngine(source, media *ColorProfile, cfg ProofConfig) (*ReferenceEngine, error) {
	if source == nil || media == nil {
		return nil, newProfileRequired(RoleSource)
	}
	if err := source.Validate(); err != nil {
		return nil, err
	}
	if err := media.Validate(); err != nil {
		return nil, err
	}
	srcM := PrimariesMatrix(source.Red, source.Green, source.Blue, source.White)
	dstM := PrimariesMatrix(media.Red, media.Green, media.Blue, media.White)
	inv := inverse3(dstM)
	if inv == ([3][3]float64{}) {
		return nil, ErrInvalidProfile
	}
	return &ReferenceEngine{source: source, media: media, cfg: cfg.withDefaults(), srcM: srcM, dstInv: inv}, nil
}

func (e *ReferenceEngine) Close() error { return nil }

func (e *ReferenceEngine) AttachDisplay(profile *LoadedProfile) error {
	if profile == nil {
		return nil
	}
	if profile.Ref.Descriptor == nil {
		return fmt.Errorf("%w: reference engine needs an explicit display color_profile; ICC display profiles require the cgo LCMS backend", ErrBackend)
	}
	if err := profile.Ref.Descriptor.Validate(); err != nil {
		return err
	}
	e.display = profile.Ref.Descriptor
	displayMatrix := PrimariesMatrix(e.display.Red, e.display.Green, e.display.Blue, e.display.White)
	e.displayInv = inverse3(displayMatrix)
	if e.displayInv == ([3][3]float64{}) {
		return ErrInvalidProfile
	}
	return nil
}

func (e *ReferenceEngine) ProofSample(_ context.Context, intent Intent, sample Sample, rgb [3]float64) (EngineOutput, error) {
	var pcs XYZ
	var sourceLab Lab
	var sourceAlarm *bool
	var absoluteXYZ *XYZ

	switch {
	case sample.Lab != nil:
		sourceLab = *sample.Lab
		pcs = LabToXYZD50(sourceLab)
		alarm := !e.sourceContainsPCS(pcs)
		sourceAlarm = &alarm
	case sample.XYZ != nil:
		pcs = *sample.XYZ
		sourceLab = XYZToLab(pcs, D50LabWhite())
		alarm := !e.sourceContainsPCS(pcs)
		sourceAlarm = &alarm
	default:
		linear := [3]float64{e.decode(rgb[0], e.source.Gamma), e.decode(rgb[1], e.source.Gamma), e.decode(rgb[2], e.source.Gamma)}
		native := matVec3(e.srcM, linear)
		nativeXYZ := XYZ{X: 100 * native[0], Y: 100 * native[1], Z: 100 * native[2]}
		absoluteXYZ = &nativeXYZ
		pcs = AdaptXYZ(nativeXYZ, e.source.White, D50XYY())
		if intent == IntentAbsoluteColorimetric {
			// Absolute intent keeps the unadapted tristimulus values so that
			// source/media white mismatch remains visible in delta E.
			sourceLab = XYZToLab(nativeXYZ, D50LabWhite())
		} else {
			sourceLab = XYZToLab(pcs, D50LabWhite())
		}
	}

	boundaryDevice := e.pcsToDevice(pcs, IntentRelativeColorimetric)
	gamutAlarm := outOfUnitRange(boundaryDevice, e.cfg.Tolerance)

	mappedLinear := e.pcsToLinear(pcs, intent)
	if intent == IntentAbsoluteColorimetric && absoluteXYZ != nil {
		mappedLinear = e.xyzToMediaLinear(*absoluteXYZ)
	}
	preEncode := [3]float64(mappedLinear)
	if intent == IntentPerceptual || intent == IntentSaturation {
		preEncode = e.compressToGamut(mappedLinear)
	}
	values := []float64{
		e.encode(preEncode[0], e.media.Gamma),
		e.encode(preEncode[1], e.media.Gamma),
		e.encode(preEncode[2], e.media.Gamma),
	}

	bits := e.cfg.OutputBitDepth
	maxCode := float64(int(1)<<bits - 1)
	codes := make([]int, 3)
	encoded := make([]float64, 3)
	for i, v := range values {
		clamped := math.Max(0, math.Min(1, v))
		code := int(math.Round(clamped * maxCode))
		codes[i] = code
		encoded[i] = float64(code) / maxCode
	}

	mediaLinear := [3]float64{e.decode(encoded[0], e.media.Gamma), e.decode(encoded[1], e.media.Gamma), e.decode(encoded[2], e.media.Gamma)}
	mediaNative := matVec3(PrimariesMatrix(e.media.Red, e.media.Green, e.media.Blue, e.media.White), mediaLinear)
	mediaNativeXYZ := XYZ{X: 100 * mediaNative[0], Y: 100 * mediaNative[1], Z: 100 * mediaNative[2]}
	var mediaLab Lab
	if intent == IntentAbsoluteColorimetric && absoluteXYZ != nil {
		mediaLab = XYZToLab(mediaNativeXYZ, D50LabWhite())
	} else {
		mediaPCS := AdaptXYZ(mediaNativeXYZ, e.media.White, D50XYY())
		mediaLab = XYZToLab(mediaPCS, D50LabWhite())
	}

	var displayValues []float64
	var displayCodes []int
	if e.display != nil {
		displayXYZ := AdaptXYZ(mediaNativeXYZ, e.media.White, e.display.White)
		displayLinear := matVec3(e.displayInv, [3]float64{displayXYZ.X / 100, displayXYZ.Y / 100, displayXYZ.Z / 100})
		maxCode := float64(int(1)<<e.cfg.OutputBitDepth - 1)
		displayValues = make([]float64, 3)
		displayCodes = make([]int, 3)
		for i := 0; i < 3; i++ {
			encoded := math.Max(0, math.Min(1, e.encode(displayLinear[i], e.display.Gamma)))
			code := int(math.Round(encoded * maxCode))
			displayValues[i] = encoded
			displayCodes[i] = code
		}
	}

	return EngineOutput{
		SourceLab: &sourceLab, SourceGamutAlarm: sourceAlarm,
		MediaColorSpace: "RGB", MediaValues: values, GamutValues: boundaryDevice,
		MediaEncoded: encoded, MediaCodes: codes, MediaLab: &mediaLab,
		MediaGamutAlarm: gamutAlarm,
		DisplayValues:   displayValues, DisplayCodes: displayCodes,
	}, nil
}

func (e *ReferenceEngine) pcsToDevice(pcs XYZ, intent Intent) []float64 {
	linear := e.pcsToLinear(pcs, intent)
	return []float64{e.encode(linear[0], e.media.Gamma), e.encode(linear[1], e.media.Gamma), e.encode(linear[2], e.media.Gamma)}
}

func (e *ReferenceEngine) pcsToLinear(pcs XYZ, intent Intent) [3]float64 {
	native := pcs
	if intent != IntentAbsoluteColorimetric {
		native = AdaptXYZ(pcs, D50XYY(), e.media.White)
	}
	return e.xyzToMediaLinear(native)
}

func (e *ReferenceEngine) xyzToMediaLinear(native XYZ) [3]float64 {
	v := [3]float64{native.X / 100, native.Y / 100, native.Z / 100}
	return matVec3(e.dstInv, v)
}

func (e *ReferenceEngine) sourceContainsPCS(pcs XYZ) bool {
	native := AdaptXYZ(pcs, D50XYY(), e.source.White)
	v := matVec3(inverse3(e.srcM), [3]float64{native.X / 100, native.Y / 100, native.Z / 100})
	for _, channel := range v {
		encoded := e.encode(channel, e.source.Gamma)
		if encoded < -e.cfg.Tolerance || encoded > 1+e.cfg.Tolerance {
			return false
		}
	}
	return true
}

func (e *ReferenceEngine) compressToGamut(rgb [3]float64) [3]float64 {
	luma := math.Max(0, math.Min(1, 0.2126*rgb[0]+0.7152*rgb[1]+0.0722*rgb[2]))
	lo, hi := 0.0, 1.0
	best := rgb
	for i := 0; i < 24; i++ {
		t := (lo + hi) / 2
		candidate := [3]float64{
			luma + (rgb[0]-luma)*(1-t),
			luma + (rgb[1]-luma)*(1-t),
			luma + (rgb[2]-luma)*(1-t),
		}
		if inUnitRange(candidate[:], e.cfg.Tolerance*10) {
			best = candidate
			hi = t
		} else {
			lo = t
		}
	}
	for i := range best {
		best[i] = math.Max(0, math.Min(1, best[i]))
	}
	return best
}

func (e *ReferenceEngine) decode(v, gamma float64) float64 {
	if v < 0 {
		return -math.Pow(-v, gamma)
	}
	return math.Pow(v, gamma)
}

func (e *ReferenceEngine) encode(v, gamma float64) float64 {
	inv := 1 / gamma
	if v < 0 {
		return -math.Pow(-v, inv)
	}
	return math.Pow(v, inv)
}

func outOfUnitRange(values []float64, tol float64) bool {
	for _, v := range values {
		if v < -tol || v > 1+tol {
			return true
		}
	}
	return false
}

func inUnitRange(values []float64, tol float64) bool {
	return !outOfUnitRange(values, tol)
}
