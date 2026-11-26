package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ManifestVersion is bumped only when the on-disk bundle layout changes
// in an incompatible way. It is part of every bundle's manifest.
const ManifestVersion = 1

// Entry is one immutable description of a file inside the bundle.
// ModTime keeps the source file's timestamp (necessary provenance
// metadata); it is normalised to whole UTC seconds.
type Entry struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	Mode      uint32    `json:"mode"`
	ModTime   time.Time `json:"modTime"`
	SHA256    string    `json:"sha256"`
	MediaKind MediaKind `json:"mediaKind"`
}

// Manifest is the canonical description of a bundle. BundleID is derived
// deterministically from the canonical JSON of the entries, never random.
type Manifest struct {
	ManifestVersion int    `json:"manifestVersion"`
	Tool            string `json:"tool"`
	BundleID        string `json:"bundleId"`
	// SourceRoot is optional provenance, omitted entirely unless the
	// caller supplies an explicit label, so a checkout path can never
	// make identical trees produce different bytes.
	SourceRoot string   `json:"sourceRoot,omitempty"`
	Entries    []*Entry `json:"entries"`
}

// Snapshot walks root and records every regular file. Directory order in
// the filesystem is irrelevant: results are sorted by canonical path.
func Snapshot(root string) ([]*Entry, error) {
	root = filepath.Clean(root)
	var entries []*Entry
	err := filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		entry, err := hashFile(root, full)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func hashFile(root, full string) (*Entry, error) {
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	h := sha256.New()
	kind, r := DetectKind(f)
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(root, full)
	if err != nil {
		return nil, err
	}
	return &Entry{
		Path:      canonicalRel(rel),
		Size:      info.Size(),
		Mode:      uint32(info.Mode().Perm()),
		ModTime:   info.ModTime().UTC().Truncate(time.Second),
		SHA256:    hex.EncodeToString(h.Sum(nil)),
		MediaKind: kind,
	}, nil
}

// canonicalRel forces slash separators and rejects traversal segments so
// the manifest is identical on every OS for the same tree.
func canonicalRel(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = path.Clean(rel)
	rel = strings.TrimPrefix(rel, "./")
	return rel
}

// CanonicalJSON renders v without any whitespace and with Go's stable
// struct field order. It is the basis of the deterministic BundleID.
func CanonicalJSON(v any) []byte {
	var b strings.Builder
	enc := jsonEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return []byte(strings.TrimSuffix(b.String(), "\n"))
}

// DeriveBundleID hashes the canonical entry list. SourceRoot and tool
// name are excluded so identical media produces identical IDs regardless
// of where the tree lives on disk.
func DeriveBundleID(entries []*Entry) string {
	sum := sha256.Sum256(CanonicalJSON(entries))
	return "sha256:" + hex.EncodeToString(sum[:])
}
