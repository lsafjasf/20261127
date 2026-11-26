package batch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

const (
	// ManifestName is the canonical, user-visible manifest file.
	ManifestName = "manifest.json"
	manifestPerm = 0o644
)

// manifestBody mirrors Manifest without the Digest field. It is the exact
// structure covered by the digest, so the digest never depends on itself.
type manifestBody struct {
	BatchID  string      `json:"batch_id"`
	SpecHash string      `json:"spec_hash"`
	Files    []FileEntry `json:"files"`
}

// BuildManifest assembles a manifest and fills in its self-digest.
func BuildManifest(batchID, specHash string, files []FileEntry) Manifest {
	ordered := append([]FileEntry(nil), files...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Path < ordered[j].Path
	})

	m := Manifest{
		BatchID:  batchID,
		SpecHash: specHash,
		Files:    ordered,
	}
	body := manifestBody{BatchID: batchID, SpecHash: specHash, Files: ordered}
	raw, _ := json.Marshal(body)
	sum := sha256.Sum256(raw)
	m.Digest = hex.EncodeToString(sum[:])
	return m
}

// VerifyDigest re-checks the manifest digest against its other fields.
func VerifyDigest(m Manifest) bool {
	body := manifestBody{BatchID: m.BatchID, SpecHash: m.SpecHash, Files: m.Files}
	raw, err := json.Marshal(body)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]) == m.Digest
}

// writeManifest atomically replaces the manifest file. A reader never sees a
// partially written manifest.
func writeManifest(dir string, m Manifest) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeAtomic(filepath.Join(dir, ManifestName), raw, manifestPerm)
}

// ReadManifest loads and structurally verifies the manifest.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return m, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&m); err != nil {
		return m, err
	}
	if !VerifyDigest(m) {
		return m, ErrCorruptManifest
	}
	return m, nil
}

// ErrCorruptManifest means a manifest's self-digest does not match its body.
var ErrCorruptManifest = batchError("manifest digest verification failed")

type batchError string

func (e batchError) Error() string { return string(e) }
