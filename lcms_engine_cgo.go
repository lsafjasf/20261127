//go:build cgo

package iccproof

/*
#cgo pkg-config: lcms2
#include <lcms2.h>
#include <stdlib.h>

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
	"fmt"
	"unsafe"
)

type lcmsEngine struct {
	source      C.cmsHPROFILE
	media       C.cmsHPROFILE
	display     C.cmsHPROFILE
	lab         C.cmsHPROFILE
	sourceMem   unsafe.Pointer
	mediaMem    unsafe.Pointer
	displayMem  unsafe.Pointer
	cfg         ProofConfig
	sourceSpace string
	mediaSpace  string
	sourceCh    int
	mediaCh     int
	bits        int
}

func newICCEngine(source, media *LoadedProfile, cfg ProofConfig) (Engine, error) {
	cfg = cfg.withDefaults()
	e := &lcmsEngine{cfg: cfg, bits: cfg.OutputBitDepth}
	if e.bits != 16 {
		e.bits = 8
	}

	var err error
	e.source, e.sourceMem, err = openLCMSProfile(source)
	if err != nil {
		return nil, err
	}
	e.media, e.mediaMem, err = openLCMSProfile(media)
	if err != nil {
		e.Close()
		return nil, err
	}
	e.lab = C.cmsCreateLab4Profile(nil)
	if e.lab == nil {
		e.Close()
		return nil, fmt.Errorf("%w: cannot create LCMS Lab PCS profile", ErrBackend)
	}

	e.sourceSpace = trimSignature(C.cmsGetColorSpace(e.source))
	e.mediaSpace = trimSignature(C.cmsGetColorSpace(e.media))
	e.sourceCh = int(C.cmsChannelsOfColorSpace(C.cmsGetColorSpace(e.source)))
	e.mediaCh = int(C.cmsChannelsOfColorSpace(C.cmsGetColorSpace(e.media)))
	if e.sourceCh <= 0 || e.mediaCh <= 0 {
		e.Close()
		return nil, fmt.Errorf("%w: unsupported ICC channel layout source=%q media=%q", ErrInvalidProfile, e.sourceSpace, e.mediaSpace)
	}
	return e, nil
}

func openLCMSProfile(p *LoadedProfile) (C.cmsHPROFILE, unsafe.Pointer, error) {
	if p == nil {
		return nil, nil, newProfileRequired("")
	}
	if p.Ref.Descriptor != nil {
		d := p.Ref.Descriptor
		white := C.cmsCIExyY{x: C.double(d.White.X), y: C.double(d.White.Y), Y: C.double(orOne(d.White.YLevel))}
		primaries := C.cmsCIExyYTRIPLE{
			Red:   C.cmsCIExyY{x: C.double(d.Red.X), y: C.double(d.Red.Y), Y: C.double(orOne(d.Red.YLevel))},
			Green: C.cmsCIExyY{x: C.double(d.Green.X), y: C.double(d.Green.Y), Y: C.double(orOne(d.Green.YLevel))},
			Blue:  C.cmsCIExyY{x: C.double(d.Blue.X), y: C.double(d.Blue.Y), Y: C.double(orOne(d.Blue.YLevel))},
		}
		gamma := C.cmsBuildGamma(nil, C.double(d.Gamma))
		if gamma == nil {
			return nil, nil, fmt.Errorf("%w: cannot build gamma curve", ErrBackend)
		}
		curves := [3]*C.cmsToneCurve{gamma, gamma, gamma}
		h := C.cmsCreateRGBProfile(&white, &primaries, (**C.cmsToneCurve)(unsafe.Pointer(&curves[0])))
		C.cmsFreeToneCurve(gamma)
		if h == nil {
			return nil, nil, fmt.Errorf("%w: cmsCreateRGBProfile failed for %q", ErrInvalidProfile, d.ID)
		}
		return h, nil, nil
	}
	if len(p.Data) == 0 {
		return nil, nil, newProfileRequired(p.Ref.Role)
	}
	mem := C.CBytes(p.Data)
	h := C.cmsOpenProfileFromMem(mem, C.cmsUInt32Number(len(p.Data)))
	if h == nil {
		C.free(mem)
		return nil, nil, &ProfileError{Role: p.Ref.Role, Path: p.Info.Path, Reason: "Little CMS rejected profile", Err: ErrInvalidProfile}
	}
	return h, mem, nil
}

func orOne(v float64) float64 {
	if v == 0 {
		return 1
	}
	return v
}

func trimSignature(sig C.cmsColorSpaceSignature) string {
	v := uint32(sig)
	raw := []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	return iccSignature(raw)
}

func (e *lcmsEngine) Close() error {
	if e.lab != nil {
		C.cmsCloseProfile(e.lab)
	}
	if e.source != nil {
		C.cmsCloseProfile(e.source)
	}
	if e.media != nil {
		C.cmsCloseProfile(e.media)
	}
	if e.display != nil {
		C.cmsCloseProfile(e.display)
	}
	if e.sourceMem != nil {
		C.free(e.sourceMem)
	}
	if e.mediaMem != nil {
		C.free(e.mediaMem)
	}
	if e.displayMem != nil {
		C.free(e.displayMem)
	}
	return nil
}

func (e *lcmsEngine) AttachDisplay(profile *LoadedProfile) error {
	h, mem, err := openLCMSProfile(profile)
	if err != nil {
		return err
	}
	if trimSignature(C.cmsGetColorSpace(h)) != "RGB" {
		C.cmsCloseProfile(h)
		if mem != nil {
			C.free(mem)
		}
		return fmt.Errorf("%w: display profile must be RGB", ErrInvalidProfile)
	}
	e.display = h
	e.displayMem = mem
	return nil
}
