//go:build cgo

package iccproof

/*
#cgo pkg-config: lcms2
#include <lcms2.h>

static cmsUInt32Number icc_fmt_rgb_dbl(void) { return TYPE_RGB_DBL; }
static cmsUInt32Number icc_fmt_rgb_8(void) { return TYPE_RGB_8; }
static cmsUInt32Number icc_fmt_rgb_16(void) { return TYPE_RGB_16; }
static cmsUInt32Number icc_fmt_cmyk_dbl(void) { return TYPE_CMYK_DBL; }
static cmsUInt32Number icc_fmt_cmyk_8(void) { return TYPE_CMYK_8; }
static cmsUInt32Number icc_fmt_cmyk_16(void) { return TYPE_CMYK_16; }
static cmsUInt32Number icc_fmt_lab_dbl(void) { return TYPE_Lab_DBL; }

static void icc_set_alarm4(cmsUInt16Number c0, cmsUInt16Number c1,
                           cmsUInt16Number c2, cmsUInt16Number c3) {
    cmsUInt16Number alarm[cmsMAXCHANNELS];
    for (cmsUInt32Number i = 0; i < cmsMAXCHANNELS; i++) alarm[i] = 0;
    alarm[0] = c0; alarm[1] = c1; alarm[2] = c2; alarm[3] = c3;
    cmsSetAlarmCodes(alarm);
}
*/
import "C"

import (
	"context"
	"fmt"
	"sync"
	"unsafe"
)

type lcmsInput struct {
	profile C.cmsHPROFILE
	format  C.cmsUInt32Number
	doubles [3]C.double
	lab     *Lab
}

func (e *lcmsEngine) ProofSample(ctx context.Context, intent Intent, sample Sample, rgb [3]float64) (EngineOutput, error) {
	if err := ctx.Err(); err != nil {
		return EngineOutput{}, err
	}
	if !intent.Valid() {
		return EngineOutput{}, fmt.Errorf("%w: %q", ErrInvalidIntent, intent)
	}

	in, err := e.resolveInput(intent, sample, rgb)
	if err != nil {
		return EngineOutput{}, err
	}

	out := EngineOutput{MediaColorSpace: e.mediaSpace, SourceLab: in.lab}

	boundary, err := e.transformMediaDoubles(in, IntentRelativeColorimetric)
	if err != nil {
		return out, err
	}
	out.GamutValues = normalizeMediaValues(boundary, e.mediaSpace)

	valuesRaw, err := e.transformMediaDoubles(in, intent)
	if err != nil {
		return out, err
	}
	out.MediaRawValues = floatSlice(valuesRaw)
	out.MediaValues = normalizeMediaValues(valuesRaw, e.mediaSpace)

	codes, encoded, err := e.transformMediaIntegers(in, intent)
	if err != nil {
		return out, err
	}
	out.MediaCodes = codes
	out.MediaEncoded = encoded

	alarm, err := e.checkMediaGamut(in)
	if err != nil {
		return out, err
	}
	out.MediaGamutAlarm = alarm

	mediaLab, err := e.codesToLab(codes, intent)
	if err != nil {
		return out, err
	}
	out.MediaLab = mediaLab
	if e.display != nil {
		displayCodes, displayValues, err := e.mediaCodesToDisplay(codes)
		if err != nil {
			return out, err
		}
		out.DisplayColorSpace = "RGB"
		out.DisplayCodes = displayCodes
		out.DisplayValues = displayValues
	}

	if sample.Lab != nil || sample.XYZ != nil {
		sourceAlarm, err := e.checkSourceGamut(in)
		if err != nil {
			return out, err
		}
		out.SourceGamutAlarm = &sourceAlarm
	}
	return out, nil
}

func (e *lcmsEngine) resolveInput(intent Intent, sample Sample, rgb [3]float64) (lcmsInput, error) {
	if sample.Lab != nil {
		return lcmsInput{
			profile: e.lab, format: C.icc_fmt_lab_dbl(),
			doubles: [3]C.double{C.double(sample.Lab.L), C.double(sample.Lab.A), C.double(sample.Lab.B)},
			lab:     sample.Lab,
		}, nil
	}
	if sample.XYZ != nil {
		lab := XYZToLab(*sample.XYZ, D50XYY().XYZ())
		return lcmsInput{
			profile: e.lab, format: C.icc_fmt_lab_dbl(),
			doubles: [3]C.double{C.double(lab.L), C.double(lab.A), C.double(lab.B)},
			lab:     &lab,
		}, nil
	}
	if e.sourceSpace != "RGB" || e.sourceCh != 3 {
		return lcmsInput{}, fmt.Errorf("%w: photo sample requires RGB source ICC, got %s/%d channels", ErrInvalidProfile, e.sourceSpace, e.sourceCh)
	}
	in := lcmsInput{
		profile: e.source, format: C.icc_fmt_rgb_dbl(),
		doubles: [3]C.double{C.double(rgb[0]), C.double(rgb[1]), C.double(rgb[2])},
	}
	lab, err := e.doublesToLab(e.source, in.format, in.doubles, C.cmsUInt32Number(intent.ICCValue()))
	if err != nil {
		return in, err
	}
	in.lab = lab
	return in, nil
}

func (e *lcmsEngine) doublesToLab(profile C.cmsHPROFILE, format C.cmsUInt32Number, in [3]C.double, intent C.cmsUInt32Number) (*Lab, error) {
	xf := C.cmsCreateTransform(profile, format, e.lab, C.icc_fmt_lab_dbl(), intent, C.cmsFLAGS_NOOPTIMIZE)
	if xf == nil {
		return nil, fmt.Errorf("%w: cannot create ICC -> Lab transform for intent %d", ErrBackend, intent)
	}
	defer C.cmsDeleteTransform(xf)
	var out [3]C.double
	C.cmsDoTransform(xf, unsafe.Pointer(&in[0]), unsafe.Pointer(&out[0]), 1)
	return &Lab{L: float64(out[0]), A: float64(out[1]), B: float64(out[2])}, nil
}

func (e *lcmsEngine) transformMediaDoubles(in lcmsInput, intent Intent) ([]C.double, error) {
	format, count := e.mediaDoubleFormat()
	out := make([]C.double, count)
	xf := C.cmsCreateTransform(in.profile, in.format, e.media, format, C.cmsUInt32Number(intent.ICCValue()), C.cmsFLAGS_NOOPTIMIZE)
	if xf == nil {
		return nil, fmt.Errorf("%w: cannot create double transform to %s for %s", ErrBackend, e.mediaSpace, intent)
	}
	defer C.cmsDeleteTransform(xf)
	C.cmsDoTransform(xf, unsafe.Pointer(&in.doubles[0]), unsafe.Pointer(&out[0]), 1)
	return out, nil
}

func (e *lcmsEngine) mediaDoubleFormat() (C.cmsUInt32Number, int) {
	if e.mediaSpace == "CMYK" {
		return C.icc_fmt_cmyk_dbl(), 4
	}
	return C.icc_fmt_rgb_dbl(), 3
}

func normalizeMediaValues(raw []C.double, space string) []float64 {
	out := make([]float64, len(raw))
	for i, v := range raw {
		out[i] = float64(v)
		if space == "CMYK" {
			out[i] /= 100
		}
	}
	return out
}

func floatSlice(in []C.double) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

func (e *lcmsEngine) mediaIntegerFormat() (C.cmsUInt32Number, float64) {
	cmyk := e.mediaSpace == "CMYK"
	if e.bits == 16 {
		if cmyk {
			return C.icc_fmt_cmyk_16(), 65535
		}
		return C.icc_fmt_rgb_16(), 65535
	}
	if cmyk {
		return C.icc_fmt_cmyk_8(), 255
	}
	return C.icc_fmt_rgb_8(), 255
}

func (e *lcmsEngine) transformMediaIntegers(in lcmsInput, intent Intent) ([]int, []float64, error) {
	format, maxCode := e.mediaIntegerFormat()
	codes := make([]int, e.mediaCh)
	xf := C.cmsCreateTransform(in.profile, in.format, e.media, format, C.cmsUInt32Number(intent.ICCValue()), C.cmsFLAGS_NOOPTIMIZE)
	if xf == nil {
		return nil, nil, fmt.Errorf("%w: cannot create integer transform to %s", ErrBackend, e.mediaSpace)
	}
	defer C.cmsDeleteTransform(xf)

	if e.bits == 16 {
		var out [16]C.cmsUInt16Number
		C.cmsDoTransform(xf, unsafe.Pointer(&in.doubles[0]), unsafe.Pointer(&out[0]), 1)
		for i := range codes {
			codes[i] = int(out[i])
		}
	} else {
		var out [16]C.cmsUInt8Number
		C.cmsDoTransform(xf, unsafe.Pointer(&in.doubles[0]), unsafe.Pointer(&out[0]), 1)
		for i := range codes {
			codes[i] = int(out[i])
		}
	}
	encoded := make([]float64, e.mediaCh)
	for i, code := range codes {
		encoded[i] = float64(code) / maxCode
	}
	return codes, encoded, nil
}

func (e *lcmsEngine) codesToLab(codes []int, intent Intent) (*Lab, error) {
	format, _ := e.mediaIntegerFormat()
	xf := C.cmsCreateTransform(e.media, format, e.lab, C.icc_fmt_lab_dbl(), C.cmsUInt32Number(intent.ICCValue()), C.cmsFLAGS_NOOPTIMIZE)
	if xf == nil {
		return nil, fmt.Errorf("%w: cannot create %s -> Lab readback transform", ErrBackend, e.mediaSpace)
	}
	defer C.cmsDeleteTransform(xf)
	var out [3]C.double
	if e.bits == 16 {
		var in [16]C.cmsUInt16Number
		for i, code := range codes {
			in[i] = C.cmsUInt16Number(code)
		}
		C.cmsDoTransform(xf, unsafe.Pointer(&in[0]), unsafe.Pointer(&out[0]), 1)
	} else {
		var in [16]C.cmsUInt8Number
		for i, code := range codes {
			in[i] = C.cmsUInt8Number(code)
		}
		C.cmsDoTransform(xf, unsafe.Pointer(&in[0]), unsafe.Pointer(&out[0]), 1)
	}
	return &Lab{L: float64(out[0]), A: float64(out[1]), B: float64(out[2])}, nil
}

func (e *lcmsEngine) media16Format() C.cmsUInt32Number {
	if e.mediaSpace == "CMYK" {
		return C.icc_fmt_cmyk_16()
	}
	return C.icc_fmt_rgb_16()
}

func (e *lcmsEngine) checkMediaGamut(in lcmsInput) (bool, error) {
	alarmMu.Lock()
	defer alarmMu.Unlock()
	normal, err := e.relativeMedia16(in)
	if err != nil {
		return false, err
	}
	candidates := [4]C.cmsUInt16Number{31415, 27182, 16180, 12345}
	alternate := [4]C.cmsUInt16Number{2718, 28182, 8281, 4590}
	for _, alarm := range [][4]C.cmsUInt16Number{candidates, alternate} {
		C.icc_set_alarm4(alarm[0], alarm[1], alarm[2], alarm[3])
		xf := C.cmsCreateProofingTransform(
			in.profile, in.format,
			e.media, e.media16Format(),
			e.media,
			C.INTENT_RELATIVE_COLORIMETRIC, C.INTENT_RELATIVE_COLORIMETRIC,
			C.cmsFLAGS_GAMUTCHECK,
		)
		if xf == nil {
			return false, fmt.Errorf("%w: cannot create gamut-check proofing transform", ErrBackend)
		}
		var out [16]C.cmsUInt16Number
		C.cmsDoTransform(xf, unsafe.Pointer(&in.doubles[0]), unsafe.Pointer(&out[0]), 1)
		C.cmsDeleteTransform(xf)
		if prefixEquals(out[:], alarm[:], e.mediaCh) && !prefixEquals(normal[:], alarm[:], e.mediaCh) {
			return true, nil
		}
	}
	return false, nil
}

func (e *lcmsEngine) relativeMedia16(in lcmsInput) ([16]C.cmsUInt16Number, error) {
	var out [16]C.cmsUInt16Number
	xf := C.cmsCreateTransform(in.profile, in.format, e.media, e.media16Format(), C.INTENT_RELATIVE_COLORIMETRIC, C.cmsFLAGS_NOOPTIMIZE)
	if xf == nil {
		return out, fmt.Errorf("%w: cannot create relative gamut reference transform", ErrBackend)
	}
	defer C.cmsDeleteTransform(xf)
	C.cmsDoTransform(xf, unsafe.Pointer(&in.doubles[0]), unsafe.Pointer(&out[0]), 1)
	return out, nil
}

func prefixEquals(values []C.cmsUInt16Number, alarm []C.cmsUInt16Number, n int) bool {
	if n > len(values) || n > len(alarm) {
		return false
	}
	for i := 0; i < n; i++ {
		if values[i] != alarm[i] {
			return false
		}
	}
	return true
}

func (e *lcmsEngine) checkSourceGamut(in lcmsInput) (bool, error) {
	alarmMu.Lock()
	defer alarmMu.Unlock()
	format := source16Format(e.sourceSpace)
	if format == 0 {
		return false, nil
	}
	alarm := [4]C.cmsUInt16Number{16180, 31415, 12345, 27182}
	C.icc_set_alarm4(alarm[0], alarm[1], alarm[2], alarm[3])
	xf := C.cmsCreateProofingTransform(
		e.lab, C.icc_fmt_lab_dbl(),
		e.source, format,
		e.source,
		C.INTENT_RELATIVE_COLORIMETRIC, C.INTENT_RELATIVE_COLORIMETRIC,
		C.cmsFLAGS_GAMUTCHECK,
	)
	if xf == nil {
		return false, fmt.Errorf("%w: cannot create source gamut-check transform", ErrBackend)
	}
	defer C.cmsDeleteTransform(xf)
	var out [16]C.cmsUInt16Number
	C.cmsDoTransform(xf, unsafe.Pointer(&in.doubles[0]), unsafe.Pointer(&out[0]), 1)
	return prefixEquals(out[:], alarm[:], e.sourceCh), nil
}

func source16Format(space string) C.cmsUInt32Number {
	switch space {
	case "RGB":
		return C.icc_fmt_rgb_16()
	case "CMYK":
		return C.icc_fmt_cmyk_16()
	default:
		return 0
	}
}

var alarmMu sync.Mutex

func (e *lcmsEngine) mediaCodesToDisplay(mediaCodes []int) ([]int, []float64, error) {
	inFormat, maxCode := e.mediaIntegerFormat()
	outFormat := C.icc_fmt_rgb_8()
	outMax := 255.0
	if e.bits == 16 {
		outFormat = C.icc_fmt_rgb_16()
		outMax = 65535
	}
	xf := C.cmsCreateTransform(e.media, inFormat, e.display, outFormat, C.INTENT_RELATIVE_COLORIMETRIC, C.cmsFLAGS_NOOPTIMIZE)
	if xf == nil {
		return nil, nil, fmt.Errorf("%w: cannot create media -> display transform", ErrBackend)
	}
	defer C.cmsDeleteTransform(xf)

	codes := make([]int, 3)
	if e.bits == 16 {
		var in [16]C.cmsUInt16Number
		for i, code := range mediaCodes {
			in[i] = C.cmsUInt16Number(code)
		}
		var out [3]C.cmsUInt16Number
		C.cmsDoTransform(xf, unsafe.Pointer(&in[0]), unsafe.Pointer(&out[0]), 1)
		for i := range codes {
			codes[i] = int(out[i])
		}
	} else {
		var in [16]C.cmsUInt8Number
		for i, code := range mediaCodes {
			in[i] = C.cmsUInt8Number(code)
		}
		var out [3]C.cmsUInt8Number
		C.cmsDoTransform(xf, unsafe.Pointer(&in[0]), unsafe.Pointer(&out[0]), 1)
		for i := range codes {
			codes[i] = int(out[i])
		}
	}
	values := make([]float64, 3)
	for i, code := range codes {
		values[i] = float64(code) / outMax
	}
	_ = maxCode
	return codes, values, nil
}
