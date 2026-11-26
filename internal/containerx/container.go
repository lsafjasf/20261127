// Package containerx inspects zip/tar containers independently of entry
// order, names and archive metadata, and writes deterministic fixtures.
package containerx

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"evidencechain/internal/digestx"
)

// Entry is one in-memory archive member used by the fixture writer.
type Entry struct {
	Name string
	Data []byte
}

// Member is one hashed archive entry.
type Member struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is the order-independent description of a container.
type Manifest struct {
	Format          string   `json:"format"`
	Members         []Member `json:"members"`
	MemberSetSHA256 string   `json:"memberset_sha256"`
	PathSetSHA256   string   `json:"pathset_sha256"`
}

// InspectFile hashes the container file and every member.
func InspectFile(path string) (fileSHA string, m *Manifest, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	fileSHA = digestx.SHA256Bytes(raw)
	format := DetectFormat(path, raw)
	switch format {
	case "zip":
		m, err = inspectZip(raw)
	case "tar":
		m, err = inspectTar(bytes.NewReader(raw), false)
	case "tar.gz":
		var gr *gzip.Reader
		gr, err = gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return "", nil, fmt.Errorf("gzip open: %w", err)
		}
		defer gr.Close()
		m, err = inspectTar(gr, true)
	default:
		return "", nil, fmt.Errorf("unsupported container format for %q", path)
	}
	if err != nil {
		return "", nil, err
	}
	m.Format = format
	m.MemberSetSHA256 = MemberSetDigest(m.Members)
	m.PathSetSHA256 = PathSetDigest(m.Members)
	return fileSHA, m, nil
}

// DetectFormat identifies zip/tar/tar.gz by extension then magic bytes.
func DetectFormat(path string, head []byte) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	}
	if len(head) >= 4 && head[0] == 'P' && head[1] == 'K' && head[2] == 3 && head[3] == 4 {
		return "zip"
	}
	if len(head) >= 262 && string(head[257:262]) == "ustar" {
		return "tar"
	}
	if len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b {
		return "tar.gz"
	}
	return "unknown"
}

func inspectZip(raw []byte) (*Manifest, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("zip open: %w", err)
	}
	m := &Manifest{Format: "zip"}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		d, err := digestx.SHA256Reader(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		m.Members = append(m.Members, Member{Name: f.Name, Size: int64(f.UncompressedSize64), SHA256: d})
	}
	return m, nil
}

func inspectTar(r io.Reader, gz bool) (*Manifest, error) {
	tr := tar.NewReader(r)
	m := &Manifest{Format: "tar"}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar read: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		d, err := digestx.SHA256Reader(tr)
		if err != nil {
			return nil, err
		}
		m.Members = append(m.Members, Member{Name: hdr.Name, Size: hdr.Size, SHA256: d})
	}
	return m, nil
}

// MemberSetDigest commits only to the sorted multiset of member content
// digests and sizes. Entry order and names cannot change it.
func MemberSetDigest(members []Member) string {
	cp := append([]Member(nil), members...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].SHA256 != cp[j].SHA256 {
			return cp[i].SHA256 < cp[j].SHA256
		}
		return cp[i].Size < cp[j].Size
	})
	h := sha256.New()
	io.WriteString(h, "EVIDENCECHAIN/MEMBERSET/v1\n")
	for _, m := range cp {
		fmt.Fprintf(h, "%s %d\n", m.SHA256, m.Size)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// PathSetDigest additionally commits to entry names, so renames change it.
func PathSetDigest(members []Member) string {
	cp := append([]Member(nil), members...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })
	h := sha256.New()
	io.WriteString(h, "EVIDENCECHAIN/PATHSET/v1\n")
	for _, m := range cp {
		fmt.Fprintf(h, "%s\x00%s\x00%d\n", m.Name, m.SHA256, m.Size)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// WriteZip writes entries in the given order with fixed metadata.
func WriteZip(path string, entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.Name)
		if err != nil {
			return err
		}
		if _, err := w.Write(e.Data); err != nil {
			return err
		}
	}
	return zw.Close()
}

// WriteTar writes an uncompressed tar in the given order.
func WriteTar(path string, entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:    e.Name,
			Mode:    0o644,
			Size:    int64(len(e.Data)),
			Uid:     1000,
			Gid:     1000,
			Uname:   "analyst",
			Gname:   "analyst",
			ModTime: fixedTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(e.Data); err != nil {
			return err
		}
	}
	return tw.Close()
}
