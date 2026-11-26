package bundle

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// ParsedBundle is a format-independent view of a packed bundle.
type ParsedBundle struct {
	Path        string
	Gzip        bool
	ManifestRaw []byte
	Manifest    map[string]any
	Legacy      bool
	BundleID    string
	PackedAt    *time.Time
	EntryOrder  []string // tar order, manifest entry first
	Entries     map[string]*Entry
	Headers     map[string]*tar.Header
}

// ReadBundle opens either a plain tar or a gzip-compressed tar and
// decodes the manifest without trusting its declared order.
func ReadBundle(path string) (*ParsedBundle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	magic, err := br.Peek(2)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	var r io.Reader = br
	gzipped := len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b
	if gzipped {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}

	pb := &ParsedBundle{
		Path:    path,
		Gzip:    gzipped,
		Entries: map[string]*Entry{},
		Headers: map[string]*tar.Header{},
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		pb.EntryOrder = append(pb.EntryOrder, hdr.Name)
		if hdr.Name == BundleManifestPath {
			pb.ManifestRaw = data
			continue
		}
		pb.Headers[hdr.Name] = hdr
	}
	if len(pb.ManifestRaw) == 0 {
		return nil, errors.New("bundle: manifest.json not found in " + path)
	}
	if err := json.Unmarshal(pb.ManifestRaw, &pb.Manifest); err != nil {
		return nil, err
	}
	if err := pb.decodeEntries(); err != nil {
		return nil, err
	}
	pb.Legacy = hasKey(pb.Manifest, "packedAt")
	pb.BundleID, _ = pb.Manifest["bundleId"].(string)
	if raw, ok := pb.Manifest["packedAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			pb.PackedAt = &t
		}
	}
	return pb, nil
}

func (pb *ParsedBundle) decodeEntries() error {
	raw, ok := pb.Manifest["entries"]
	if !ok {
		return errors.New("bundle: manifest has no entries")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var entries []*Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	for _, e := range entries {
		pb.Entries[e.Path] = e
	}
	return nil
}

// ManifestOrder returns entry paths in the order declared by the
// manifest itself (as opposed to the tar container order).
func (pb *ParsedBundle) ManifestOrder() []string {
	raw, ok := pb.Manifest["entries"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if p, ok := m["path"].(string); ok {
			out = append(out, p)
		}
	}
	return out
}

// GzipModTime returns the gzip header ModTime when the bundle is gzipped.
func (pb *ParsedBundle) GzipModTime() *time.Time {
	if !pb.Gzip {
		return nil
	}
	// Re-open briefly: the gzip header is not retained while streaming.
	f, err := os.Open(pb.Path)
	if err != nil {
		return nil
	}
	defer f.Close()
	br := bufio.NewReader(f)
	gz, err := gzip.NewReader(br)
	if err != nil {
		return nil
	}
	defer gz.Close()
	if gz.ModTime.IsZero() {
		return nil
	}
	t := gz.ModTime.UTC()
	return &t
}

func hasKey(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}
