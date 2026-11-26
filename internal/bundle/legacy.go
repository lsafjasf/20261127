package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"io"
	"math/big"
	"os"
	"time"
)

// LegacyManifest mirrors what many real-world packers emit: a random
// bundle id and a wall-clock packedAt field. Source metadata is still
// fully retained — the point is to isolate the non-determinism sources,
// not to strip necessary information.
type LegacyManifest struct {
	ManifestVersion int       `json:"manifestVersion"`
	Tool            string    `json:"tool"`
	BundleID        string    `json:"bundleId"`
	PackedAt        time.Time `json:"packedAt"`
	SourceRoot      string    `json:"sourceRoot,omitempty"`
	Entries         []*Entry  `json:"entries"`
}

// LegacyPack intentionally reproduces the three classic non-determinism
// sources so the diff classifier can be exercised end to end:
//   - random BundleID (random ID);
//   - shuffled entry order (container order);
//   - current-time PackedAt and gzip ModTime (timestamp).
func LegacyPack(srcDir, out string, opts Options) (*LegacyManifest, error) {
	entries, err := Snapshot(srcDir)
	if err != nil {
		return nil, err
	}
	shuffled := append([]*Entry(nil), entries...)
	shuffle(shuffled)

	id, err := randomID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	manifest := &LegacyManifest{
		ManifestVersion: ManifestVersion,
		Tool:            ToolName + "-legacy",
		BundleID:        id,
		PackedAt:        now,
		SourceRoot:      opts.SourceRoot,
		Entries:         shuffled,
	}
	if err := writeLegacy(out, opts.Compression, manifest, srcDir, now); err != nil {
		return nil, err
	}
	return manifest, nil
}

func writeLegacy(out, compression string, manifest *LegacyManifest, srcDir string, now time.Time) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(out)
		}
	}()

	var w io.WriteCloser = f
	if compression == "gzip" {
		gz := gzip.NewWriter(f)
		gz.ModTime = now
		w = gz
	}

	tw := tar.NewWriter(w)
	data := CanonicalJSON(manifest)
	hdr := &tar.Header{
		Name:     BundleManifestPath,
		Mode:     0o644,
		ModTime:  now,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	for _, e := range manifest.Entries {
		if err := writeFile(tw, srcDir, e); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if compression == "gzip" {
		if err := w.Close(); err != nil {
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func shuffle(s []*Entry) {
	for i := len(s) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			panic(err)
		}
		j := int(n.Int64())
		s[i], s[j] = s[j], s[i]
	}
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	// RFC 4122 version 4 / variant 10xx, formatted with hyphens.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	return "urn:uuid:" + s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
