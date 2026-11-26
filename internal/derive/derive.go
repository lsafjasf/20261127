// Package derive implements reproducible derivation transforms and
// content-level comparisons used by provenance verification.
package derive

import (
	"bytes"
	"fmt"
	"image/png"
	"strings"
)

// NormalizeEOL reproduces the working-copy text normalization:
// CRLF/CR -> LF, strip trailing spaces and tabs per line, final newline.
func NormalizeEOL(data []byte) []byte {
	s := string(data)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	s = strings.Join(lines, "\n")
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s)
}

// PNGPixelsEqual reports whether two PNG byte streams decode to identical
// pixel grids. Different compression/metadata therefore does not count as
// different visual evidence.
func PNGPixelsEqual(a, b []byte) (bool, string, error) {
	ia, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		return false, "", fmt.Errorf("decode first png: %w", err)
	}
	ib, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return false, "", fmt.Errorf("decode second png: %w", err)
	}
	if ia.Bounds() != ib.Bounds() {
		return false, fmt.Sprintf("bounds %v vs %v", ia.Bounds(), ib.Bounds()), nil
	}
	r := ia.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			ca := ia.At(x, y)
			cb := ib.At(x, y)
			ar, ag, ab, aa := ca.RGBA()
			br, bg, bb, ba := cb.RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				return false, fmt.Sprintf("first pixel difference at (%d,%d)", x, y), nil
			}
		}
	}
	return true, "解码像素逐点完全一致", nil
}
