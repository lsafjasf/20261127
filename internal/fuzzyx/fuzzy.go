// Package fuzzyx implements a small similarity (fuzzy) digest.
//
// It is deliberately NOT a cryptographic identifier: two objects with a
// close SimHash or a high shingle overlap are merely "similar". Similarity
// can support a lead ("compare these two manually") but can never prove
// byte-identity or a common source by itself.
package fuzzyx

import (
	"fmt"
	"hash/fnv"
	"math/bits"
	"strconv"
	"strings"
)

// ShingleSize is the sliding byte-window size for feature extraction.
const ShingleSize = 8

// Digest is a similarity fingerprint plus overlap statistics helpers.
type Digest struct {
	Algorithm string `json:"algorithm"`
	ShingleK  int    `json:"shingle_k"`
	Features  int    `json:"features"`
	Unique    int    `json:"unique"`
	SimHash   string `json:"simhash"`
}

// Result compares two objects using similarity-only signals.
type Result struct {
	JaccardPercent float64 `json:"jaccard_percent"`
	HammingBits    int     `json:"hamming_bits"`
	HashBits       int     `json:"hash_bits"`
}

func featureHashes(b []byte, k int) []uint64 {
	if k <= 0 {
		k = ShingleSize
	}
	if len(b) == 0 {
		return nil
	}
	if len(b) < k {
		k = len(b)
	}
	n := len(b) - k + 1
	out := make([]uint64, 0, n)
	for i := 0; i < n; i++ {
		h := fnv.New64a()
		h.Write(b[i : i+k])
		out = append(out, h.Sum64())
	}
	return out
}

// SimHash computes the 64-bit weighted SimHash over byte shingles.
func SimHash(b []byte) uint64 {
	hs := featureHashes(b, ShingleSize)
	var bitsV [64]int
	for _, h := range hs {
		for bit := 0; bit < 64; bit++ {
			if h&(uint64(1)<<bit) != 0 {
				bitsV[bit]++
			} else {
				bitsV[bit]--
			}
		}
	}
	var fp uint64
	for bit := 0; bit < 64; bit++ {
		if bitsV[bit] > 0 {
			fp |= uint64(1) << bit
		}
	}
	return fp
}

// Sum builds the serializable fuzzy digest of b.
func Sum(b []byte) Digest {
	hs := featureHashes(b, ShingleSize)
	seen := make(map[uint64]struct{}, len(hs))
	for _, h := range hs {
		seen[h] = struct{}{}
	}
	fp := SimHash(b)
	return Digest{
		Algorithm: "ecsh1",
		ShingleK:  ShingleSize,
		Features:  len(hs),
		Unique:    len(seen),
		SimHash:   fmt.Sprintf("%016x", fp),
	}
}

// ParseSimHash parses the trailing 16-hex SimHash from a digest string or
// accepts a bare 16-hex string.
func ParseSimHash(s string) (uint64, error) {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[i+1:]
	}
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("bad simhash %q: %w", s, err)
	}
	return v, nil
}

// Compare computes overlap and SimHash distance between two byte slices.
func Compare(a, b []byte) Result {
	ha := featureHashes(a, ShingleSize)
	hb := featureHashes(b, ShingleSize)
	sa := make(map[uint64]struct{}, len(ha))
	for _, h := range ha {
		sa[h] = struct{}{}
	}
	sb := make(map[uint64]struct{}, len(hb))
	for _, h := range hb {
		sb[h] = struct{}{}
	}
	inter := 0
	for h := range sa {
		if _, ok := sb[h]; ok {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	j := 0.0
	if union > 0 {
		j = float64(inter) * 100 / float64(union)
	}
	dist := bits.OnesCount64(SimHash(a) ^ SimHash(b))
	return Result{JaccardPercent: round1(j), HammingBits: dist, HashBits: 64}
}

// CompareHash compares bytes against a previously recorded SimHash.
func CompareHash(b []byte, simhashHex string) (Result, error) {
	want, err := ParseSimHash(simhashHex)
	if err != nil {
		return Result{}, err
	}
	got := SimHash(b)
	return Result{HammingBits: bits.OnesCount64(got ^ want), HashBits: 64}, nil
}

// SimilarityLeadThreshold is the minimum unique-shingle Jaccard (percent) at
// which the tool raises a "similarity lead". With 8-byte shingles random
// data scores ~0 and shared boilerplate/template text scores roughly 20-40,
// so 25 is a deliberate "worth human comparison" line. Crossing it never
// implies identity or common source.
const SimilarityLeadThreshold = 25.0

// IsLead reports whether r warrants manual similarity review.
func (r Result) IsLead() bool { return r.JaccardPercent >= SimilarityLeadThreshold }

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
