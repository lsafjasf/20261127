package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sort"
	"strconv"
)

// Verdict values returned by Compare.
const (
	// VerdictIdentical: byte-for-byte identical containers.
	VerdictIdentical = "identical"
	// VerdictCosmeticEquivalent: same media and same necessary metadata;
	// differences are limited to timestamps, container order or random ids.
	VerdictCosmeticEquivalent = "cosmetic-equivalent"
	// VerdictMetadataDifferent: media bytes identical but necessary source
	// metadata (mtime/mode) differs.
	VerdictMetadataDifferent = "metadata-different"
	// VerdictMediaDifferent: one or more payload files were added, removed
	// or have different content hashes.
	VerdictMediaDifferent = "media-different"
)

// FieldDiff records one changed scalar field of one entry.
type FieldDiff struct {
	Path  string `json:"path"`
	Field string `json:"field"`
	A     string `json:"a"`
	B     string `json:"b"`
}

// MediaDiff records a real payload-content difference.
type MediaDiff struct {
	Path   string    `json:"path"`
	Kind   string    `json:"kind"` // "content" | "added-in-b" | "removed-in-b"
	AHash  string    `json:"aHash,omitempty"`
	BHash  string    `json:"bHash,omitempty"`
	AMedia MediaKind `json:"aMediaKind,omitempty"`
	BMedia MediaKind `json:"bMediaKind,omitempty"`
}

// TimestampDiff records one packaging-time difference (not source mtime).
type TimestampDiff struct {
	Where string `json:"where"` // "manifest.packedAt" | "gzip.modTime" | "tar.manifest.modTime"
	A     string `json:"a"`
	B     string `json:"b"`
}

// Report is the structured result of comparing two bundles.
type Report struct {
	Verdict             string          `json:"verdict"`
	RawIdentical        bool            `json:"rawIdentical"`
	MediaEquivalent     bool            `json:"mediaEquivalent"`
	BundleIDA           string          `json:"bundleIdA"`
	BundleIDB           string          `json:"bundleIdB"`
	ContainerOrderDiff  bool            `json:"containerOrderDiff"`
	BundleIDDifference  bool            `json:"bundleIdDifference"`
	PackagingTimestamps []TimestampDiff `json:"packagingTimestampDiffs"`
	SourceMetadataDiffs []FieldDiff     `json:"sourceMetadataDiffs"`
	MediaDiffs          []MediaDiff     `json:"mediaDiffs"`
}

// Compare parses two bundles and explains every axis on which they differ.
func Compare(pathA, pathB string) (*Report, error) {
	a, err := ReadBundle(pathA)
	if err != nil {
		return nil, err
	}
	b, err := ReadBundle(pathB)
	if err != nil {
		return nil, err
	}

	hashA, err := fileHash(pathA)
	if err != nil {
		return nil, err
	}
	hashB, err := fileHash(pathB)
	if err != nil {
		return nil, err
	}

	r := &Report{
		RawIdentical:        hashA == hashB,
		BundleIDA:           a.BundleID,
		BundleIDB:           b.BundleID,
		PackagingTimestamps: []TimestampDiff{},
		SourceMetadataDiffs: []FieldDiff{},
		MediaDiffs:          []MediaDiff{},
	}
	r.BundleIDDifference = a.BundleID != b.BundleID
	r.ContainerOrderDiff = !equalStrings(payloadOrder(a), payloadOrder(b))
	r.PackagingTimestamps = comparePackagingTimestamps(a, b)
	compareEntries(a, b, r)

	r.MediaEquivalent = len(r.MediaDiffs) == 0
	r.Verdict = decide(r)
	return r, nil
}

func decide(r *Report) string {
	switch {
	case r.RawIdentical:
		return VerdictIdentical
	case len(r.MediaDiffs) > 0:
		return VerdictMediaDifferent
	case len(r.SourceMetadataDiffs) > 0:
		return VerdictMetadataDifferent
	default:
		return VerdictCosmeticEquivalent
	}
}

// payloadOrder drops the manifest entry, whose position is fixed.
func payloadOrder(pb *ParsedBundle) []string {
	out := make([]string, 0, len(pb.EntryOrder))
	for _, name := range pb.EntryOrder {
		if name != BundleManifestPath {
			out = append(out, name)
		}
	}
	return out
}

func comparePackagingTimestamps(a, b *ParsedBundle) []TimestampDiff {
	var diffs []TimestampDiff
	pa, pb := "-", "-"
	if a.PackedAt != nil {
		pa = a.PackedAt.Format(rfc3339UTC)
	}
	if b.PackedAt != nil {
		pb = b.PackedAt.Format(rfc3339UTC)
	}
	if pa != pb {
		diffs = append(diffs, TimestampDiff{Where: "manifest.packedAt", A: pa, B: pb})
	}

	ga, gb := "-", "-"
	if t := a.GzipModTime(); t != nil {
		ga = t.Format(rfc3339UTC)
	}
	if t := b.GzipModTime(); t != nil {
		gb = t.Format(rfc3339UTC)
	}
	if ga != gb {
		diffs = append(diffs, TimestampDiff{Where: "gzip.modTime", A: ga, B: gb})
	}

	ma, mb := "-", "-"
	if h, ok := a.Headers[BundleManifestPath]; ok {
		ma = h.ModTime.Format(rfc3339UTC)
	}
	if h, ok := b.Headers[BundleManifestPath]; ok {
		mb = h.ModTime.Format(rfc3339UTC)
	}
	if ma != mb {
		diffs = append(diffs, TimestampDiff{Where: "tar.manifest.modTime", A: ma, B: mb})
	}
	return diffs
}

const rfc3339UTC = "2006-01-02T15:04:05Z"

func compareEntries(a, b *ParsedBundle, r *Report) {
	paths := unionPaths(a.Entries, b.Entries)
	for _, p := range paths {
		ea, inA := a.Entries[p]
		eb, inB := b.Entries[p]
		switch {
		case inA && !inB:
			r.MediaDiffs = append(r.MediaDiffs, MediaDiff{
				Path: p, Kind: "removed-in-b", AHash: ea.SHA256, AMedia: ea.MediaKind,
			})
		case !inA && inB:
			r.MediaDiffs = append(r.MediaDiffs, MediaDiff{
				Path: p, Kind: "added-in-b", BHash: eb.SHA256, BMedia: eb.MediaKind,
			})
		default:
			if ea.SHA256 != eb.SHA256 {
				r.MediaDiffs = append(r.MediaDiffs, MediaDiff{
					Path: p, Kind: "content",
					AHash: ea.SHA256, BHash: eb.SHA256,
					AMedia: ea.MediaKind, BMedia: eb.MediaKind,
				})
				continue
			}
			if !ea.ModTime.Equal(eb.ModTime) {
				r.SourceMetadataDiffs = append(r.SourceMetadataDiffs, FieldDiff{
					Path: p, Field: "modTime",
					A: ea.ModTime.Format(rfc3339UTC), B: eb.ModTime.Format(rfc3339UTC),
				})
			}
			if ea.Mode != eb.Mode {
				r.SourceMetadataDiffs = append(r.SourceMetadataDiffs, FieldDiff{
					Path: p, Field: "mode",
					A: octal(ea.Mode), B: octal(eb.Mode),
				})
			}
		}
	}
}

func unionPaths(x, y map[string]*Entry) []string {
	seen := map[string]struct{}{}
	for p := range x {
		seen[p] = struct{}{}
	}
	for p := range y {
		seen[p] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func octal(m uint32) string {
	return "0" + strconv.FormatUint(uint64(m), 8)
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
