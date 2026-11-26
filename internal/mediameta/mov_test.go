package mediameta

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestScanProResAlphaMarker(t *testing.T) {
	data := make([]byte, 64)
	copy(data[4:8], "icpf")
	data[4+4+17] = 2

	var info MOVInfo
	scanProResBytes(data, &info)
	if info.ICPFCount != 1 {
		t.Fatalf("icpf count = %d", info.ICPFCount)
	}
	if len(info.ProResAlphaTypes) != 1 || info.ProResAlphaTypes[0] != 2 {
		t.Fatalf("alpha types = %v", info.ProResAlphaTypes)
	}
}

func TestParseMOVColorBox(t *testing.T) {
	colorBox := make([]byte, 18)
	binary.BigEndian.PutUint32(colorBox[0:4], uint32(len(colorBox)))
	copy(colorBox[4:8], "colr")
	copy(colorBox[8:12], "nclc")
	binary.BigEndian.PutUint16(colorBox[12:14], 1)
	binary.BigEndian.PutUint16(colorBox[14:16], 1)
	binary.BigEndian.PutUint16(colorBox[16:18], 1)

	moov := make([]byte, 8+len(colorBox))
	binary.BigEndian.PutUint32(moov[0:4], uint32(len(moov)))
	copy(moov[4:8], "moov")
	copy(moov[8:], colorBox)

	path := filepath.Join(t.TempDir(), "color.mov")
	if err := os.WriteFile(path, moov, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := parseMOVFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.ColorBox == nil {
		t.Fatal("colr box not found")
	}
	if info.ColorBox.ColorType != "nclc" || info.ColorBox.Primaries != 1 ||
		info.ColorBox.Transfer != 1 || info.ColorBox.Matrix != 1 {
		t.Fatalf("unexpected colr: %+v", info.ColorBox)
	}
	if info.ColorBox.PrimariesName != "bt709" {
		t.Fatalf("primaries name = %q", info.ColorBox.PrimariesName)
	}
}
