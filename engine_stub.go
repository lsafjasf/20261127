//go:build !cgo

package iccproof

import "errors"

var ErrCGODisabled = errors.New("ICC backend requires cgo with liblcms2; rebuild with CGO_ENABLED=1 or supply explicit color_profile descriptors")

func newICCEngine(source, media *LoadedProfile, cfg ProofConfig) (Engine, error) {
	return nil, ErrCGODisabled
}
