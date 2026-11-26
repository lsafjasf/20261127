package main

import (
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"strings"
)

// SigKind identifies how a frame visual signature should be compared.
type SigKind int

const (
	SigNone SigKind = iota
	// SigPHash64 is a 64-bit perceptual/difference hash; distance is Hamming/64.
	SigPHash64
	// SigExact is an exact content token (MD5/SHA/...); equal or different only.
	SigExact
)

// Sig is a compact visual fingerprint of one frame.
type Sig struct {
	Kind SigKind
	P    uint64
	E    string
}

// Valid reports whether a signature is present.
func (s Sig) Valid() bool { return s.Kind != SigNone }

// ParseSig accepts pHash/dHash/aHash hex (16 chars), 64-char bit strings,
// or any longer hex digest such as MD5/SHA. Prefixes like "phash:" are allowed.
func ParseSig(raw string) (Sig, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.EqualFold(s, "null") || s == "-" {
		return Sig{}, nil
	}
	if i := strings.IndexByte(s, ':'); i >= 0 && i < 12 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	if len(s) == 64 && isBitString(s) {
		var v uint64
		for i := 0; i < 64; i++ {
			if s[i] == '1' {
				v |= 1 << (63 - i)
			}
		}
		return Sig{Kind: SigPHash64, P: v}, nil
	}
	if b, err := hex.DecodeString(s); err == nil && len(b) == 8 {
		return Sig{Kind: SigPHash64, P: binary.BigEndian.Uint64(b)}, nil
	}
	if _, err := hex.DecodeString(s); err == nil && len(s)%2 == 0 && len(s) >= 4 {
		return Sig{Kind: SigExact, E: strings.ToLower(s)}, nil
	}
	// Non-hex token: still usable for exact equality.
	return Sig{Kind: SigExact, E: "tok:" + s}, nil
}

func isBitString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' && s[i] != '1' {
			return false
		}
	}
	return true
}

// SigDistance returns normalized visual distance in [0,1].
// Returns -1 when the two signatures cannot be compared.
func SigDistance(a, b Sig) float64 {
	if a.Kind == SigNone || b.Kind == SigNone {
		return -1
	}
	if a.Kind == SigPHash64 && b.Kind == SigPHash64 {
		return float64(bits.OnesCount64(a.P^b.P)) / 64.0
	}
	if a.Kind == SigExact && b.Kind == SigExact {
		if a.E == b.E {
			return 0
		}
		return 1
	}
	return -1
}

// SigSame reports whether two signatures count as "the same picture"
// under dupBits tolerance for perceptual hashes.
func SigSame(a, b Sig, dupBits int) bool {
	if a.Kind == SigNone || b.Kind == SigNone {
		return false
	}
	if a.Kind == SigPHash64 && b.Kind == SigPHash64 {
		return bits.OnesCount64(a.P^b.P) <= dupBits
	}
	return SigDistance(a, b) == 0
}
