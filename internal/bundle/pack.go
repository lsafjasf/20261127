package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ToolName identifies this implementation inside manifests.
const ToolName = "mediabundle"

// BundleManifestPath is the fixed, first entry of every bundle.
const BundleManifestPath = "manifest.json"

// Options controls packaging. Compression "gzip" uses a fully
// deterministic gzip wrapper (zero ModTime, default name/comment empty).
type Options struct {
	Compression string // "" or "gzip"
	SourceRoot  string // recorded verbatim for provenance
}

// Pack builds a reproducible bundle from srcDir into out.
//
// Reproducibility guarantees (see SPEC.md for the full contract):
//   - entries are sorted by path;
//   - no wall-clock time is read at pack time;
//   - the manifest header carries the fixed epoch time;
//   - tar Uid/Gid/Uname/Gname/Devmajor/Devminor are zeroed;
//   - the bundle id is a content hash, not a random UUID;
//   - gzip headers are empty and ModTime is zero.
func Pack(srcDir, out string, opts Options) (*Manifest, error) {
	entries, err := Snapshot(srcDir)
	if err != nil {
		return nil, err
	}
	manifest := &Manifest{
		ManifestVersion: ManifestVersion,
		Tool:            ToolName,
		SourceRoot:      opts.SourceRoot,
		Entries:         entries,
		BundleID:        DeriveBundleID(entries),
	}
	if err := writeBundle(out, opts.Compression, manifest, srcDir, entries); err != nil {
		return nil, err
	}
	return manifest, nil
}

func writeBundle(out, compression string, manifest *Manifest, srcDir string, entries []*Entry) error {
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
		gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
		if err != nil {
			return err
		}
		gz.ModTime = time.Time{}
		gz.Name = ""
		gz.Comment = ""
		w = gz
	}

	tw := tar.NewWriter(w)
	if err := writeManifest(tw, manifest); err != nil {
		return err
	}
	for _, e := range entries {
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

// fixedEpoch is the timestamp used for the manifest entry itself. It is
// deliberately constant; the manifest already carries source ModTimes.
var fixedEpoch = time.Unix(0, 0).UTC()

func writeManifest(tw *tar.Writer, manifest *Manifest) error {
	data := CanonicalJSON(manifest)
	hdr := &tar.Header{
		Name:     BundleManifestPath,
		Mode:     0o644,
		ModTime:  fixedEpoch,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func writeFile(tw *tar.Writer, srcDir string, e *Entry) error {
	full := filepath.Join(srcDir, filepath.FromSlash(e.Path))
	hdr := &tar.Header{
		Name:       e.Path,
		Mode:       int64(e.Mode),
		ModTime:    e.ModTime,
		Size:       e.Size,
		Typeflag:   tar.TypeReg,
		Format:     tar.FormatPAX,
		AccessTime: time.Time{},
		ChangeTime: time.Time{},
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()
	sum, err := io.Copy(tw, f)
	if err != nil {
		return err
	}
	if sum != e.Size {
		return fmt.Errorf("bundle: size mismatch for %s: header %d wrote %d", e.Path, e.Size, sum)
	}
	return nil
}

// HashFile returns the lowercase hex SHA-256 of a bundle file.
func HashFile(path string) (string, error) {
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
