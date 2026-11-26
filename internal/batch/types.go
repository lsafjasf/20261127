// Package batch implements a crash-safe, resumable batch rendering engine
// with staged fragment output, an append-only transaction log and an
// atomically committed manifest.
package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Spec describes a deterministic batch rendering job.
type Spec struct {
	Name  string     `json:"name"`
	Files []FileSpec `json:"files"`
}

// FileSpec describes one rendered output and its fragment split.
type FileSpec struct {
	// Path is the output path relative to the files directory.
	Path string `json:"path"`
	// Fragments is the number of independently staged fragments.
	Fragments int `json:"fragments"`
	// FragmentSize is the byte size of every fragment except possibly the
	// last one; the renderer expands the last fragment to Size.
	FragmentSize int `json:"fragment_size"`
	// Size is the total deterministic output size in bytes.
	Size int `json:"size"`
}

// FragmentEntry records the committed identity of one fragment.
type FragmentEntry struct {
	Index  int    `json:"index"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// FileEntry is one row of the committed manifest.
type FileEntry struct {
	Path      string          `json:"path"`
	Size      int64           `json:"size"`
	SHA256    string          `json:"sha256"`
	Fragments []FragmentEntry `json:"fragments"`
}

// Manifest is the committed result of a batch. Its Digest covers the
// canonical JSON encoding of every other field, so two identical batches
// always produce byte-identical manifests.
type Manifest struct {
	BatchID  string      `json:"batch_id"`
	SpecHash string      `json:"spec_hash"`
	Files    []FileEntry `json:"files"`
	Digest   string      `json:"digest"`
}

// CanonicalSpecHash hashes the canonical, key-stable JSON encoding of a spec.
func CanonicalSpecHash(s Spec) (string, error) {
	raw, err := json.Marshal(normalizeSpec(s))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// normalizeSpec returns a value with files sorted by path so that spec
// ordering cannot perturb hashes.
func normalizeSpec(s Spec) Spec {
	out := Spec{Name: s.Name, Files: append([]FileSpec(nil), s.Files...)}
	for i := range out.Files {
		if out.Files[i].Fragments <= 0 {
			out.Files[i].Fragments = 1
		}
	}
	for i := 1; i < len(out.Files); i++ {
		for j := i; j > 0 && out.Files[j-1].Path > out.Files[j].Path; j-- {
			out.Files[j-1], out.Files[j] = out.Files[j], out.Files[j-1]
		}
	}
	return out
}

// fileState is the in-memory projection rebuilt by replaying the WAL.
type fileState struct {
	fragments map[int]FragmentEntry
	fileHash  string
	size      int64
	committed bool
}

func newFileState() *fileState {
	return &fileState{fragments: map[int]FragmentEntry{}}
}
