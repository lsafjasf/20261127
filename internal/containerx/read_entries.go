package containerx

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"evidencechain/internal/digestx"
)

// ReadEntries loads every regular member together with its bytes. The
// reference case is small; real large cases should use InspectFile streaming
// plus targeted extraction instead.
func ReadEntries(path string) (format string, fileSHA string, entries []Entry, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", nil, err
	}
	fileSHA = digestx.SHA256Bytes(raw)
	format = DetectFormat(path, raw)
	switch format {
	case "zip":
		entries, err = readZipEntries(raw)
	case "tar":
		entries, err = readTarEntries(bytes.NewReader(raw))
	case "tar.gz":
		var gr *gzip.Reader
		gr, err = gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return "", "", nil, fmt.Errorf("gzip open: %w", err)
		}
		defer gr.Close()
		entries, err = readTarEntries(gr)
	default:
		return "", "", nil, fmt.Errorf("unsupported container format for %q", filepath.Base(path))
	}
	return format, fileSHA, entries, err
}

func readZipEntries(raw []byte) ([]Entry, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, rerr := io.ReadAll(rc)
		rc.Close()
		if rerr != nil {
			return nil, rerr
		}
		out = append(out, Entry{Name: f.Name, Data: data})
	}
	return out, nil
}

func readTarEntries(r io.Reader) ([]Entry, error) {
	tr := tar.NewReader(r)
	var out []Entry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
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
		out = append(out, Entry{Name: strings.TrimPrefix(hdr.Name, "./"), Data: data})
	}
	return out, nil
}

// IsContainerPath reports whether path looks like a supported archive.
func IsContainerPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar") ||
		strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

// MembersFromEntries converts entries to hash-only Members.
func MembersFromEntries(entries []Entry) []Member {
	out := make([]Member, 0, len(entries))
	for _, e := range entries {
		out = append(out, Member{Name: e.Name, Size: int64(len(e.Data)), SHA256: digestx.SHA256Bytes(e.Data)})
	}
	return out
}
