// Package casegen deterministically builds the synthetic reference case:
// sealed originals, verified derived copies, renamed/reordered/tampered
// field returns, external watchlist hashes and the provenance graph.
package casegen

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"

	"evidencechain/internal/containerx"
	"evidencechain/internal/derive"
)

// CaseID is the stable identifier of the worked example.
const CaseID = "CASE-2026-1127"

type fixture struct {
	dir      string
	ev1      []byte
	ev2      []byte
	ev3      []byte
	ev4      []byte
	tampered []byte
	parody   []byte
	unknown  []byte
}

var originalNames = map[string]string{
	"EV-001": "EV-001_值班日志_20261120.txt",
	"EV-002": "EV-002_客户名单.csv",
	"EV-003": "EV-003_设备固件片段.bin",
	"EV-004": "EV-004_出入口抓拍.png",
}

// Generate materializes the full reference case under dir.
func Generate(dir string) error {
	f := &fixture{dir: dir}
	f.ev1 = ev001LogWindows()
	f.ev2 = ev002CustomersCSV()
	f.ev3 = ev003FirmwareBlob()
	png4, err := ev004CapturePNG(png.BestCompression)
	if err != nil {
		return err
	}
	f.ev4 = png4
	f.tampered = ev001Tampered(f.ev1)
	f.parody = parodyLog()
	f.unknown = unknownCacheBlob()

	for _, d := range []string{
		"00-originals", "10-derived", "20-containers",
		"30-field-returns", "watchlist", "custody",
		"review", "index", "out",
	} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}

	if err := f.writeOriginals(); err != nil {
		return err
	}
	if err := f.writeDerived(); err != nil {
		return err
	}
	if err := f.writeSealedContainer(); err != nil {
		return err
	}
	if err := f.writeFieldReturns(); err != nil {
		return err
	}
	if err := f.writeWatchlist(); err != nil {
		return err
	}
	return f.writeProvenance()
}

func (f *fixture) writeOriginals() error {
	data := map[string][]byte{
		"EV-001": f.ev1, "EV-002": f.ev2, "EV-003": f.ev3, "EV-004": f.ev4,
	}
	for id, name := range originalNames {
		if err := os.WriteFile(filepath.Join(f.dir, "00-originals", name), data[id], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (f *fixture) writeDerived() error {
	norm := derive.NormalizeEOL(f.ev1)
	reenc, err := reencodePNG(f.ev4)
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"EV-001.字节副本.txt":  f.ev1,
		"EV-001.规范化.txt":   norm,
		"EV-002.字节副本.csv":  f.ev2,
		"EV-004.像素重编码.png": reenc,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(f.dir, "10-derived", name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (f *fixture) writeSealedContainer() error {
	entries := []containerx.Entry{
		{Name: originalNames["EV-001"], Data: f.ev1},
		{Name: originalNames["EV-002"], Data: f.ev2},
		{Name: originalNames["EV-003"], Data: f.ev3},
		{Name: originalNames["EV-004"], Data: f.ev4},
	}
	return containerx.WriteZip(filepath.Join(f.dir, "20-containers", "sealed-pack.zip"), entries)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	return nil
}

func rel(p ...string) string { return filepath.ToSlash(filepath.Join(p...)) }
