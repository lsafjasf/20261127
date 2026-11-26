package iccproof

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestReadJPEGICCReassemblesSegments(t *testing.T) {
	first := []byte("first-part-")
	second := []byte("second-part")
	path := filepath.Join(t.TempDir(), "photo.jpg")
	var data []byte
	data = append(data, 0xff, 0xd8)
	data = appendJPEGICCChunk(t, data, 1, 2, first)
	data = appendJPEGICCChunk(t, data, 2, 2, second)
	data = append(data, 0xff, 0xd9)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEmbeddedICC(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(append(first, second...)) {
		t.Fatalf("ICC = %q", got)
	}
}

func appendJPEGICCChunk(t *testing.T, data []byte, seq, total int, payload []byte) []byte {
	t.Helper()
	body := append([]byte("ICC_PROFILE\x00"), byte(seq), byte(total))
	body = append(body, payload...)
	segmentLength := make([]byte, 2)
	binary.BigEndian.PutUint16(segmentLength, uint16(len(body)+2))
	data = append(data, 0xff, 0xe2)
	data = append(data, segmentLength...)
	return append(data, body...)
}

func TestReadPNGICCDecompressesICCP(t *testing.T) {
	icc := []byte("0123456789acsp-test-profile")
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(icc); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte("demo\x00\x00"), compressed.Bytes()...)
	chunk := []byte("iCCP")
	chunk = append(chunk, payload...)
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(payload)))
	crc := crc32.ChecksumIEEE(chunk)
	crcBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(crcBytes, crc)

	var png []byte
	png = append(png, 0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n')
	png = append(png, length...)
	// chunk currently starts with type; length must precede it.
	png = append(png, chunk...)
	png = append(png, crcBytes...)
	path := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadEmbeddedICC(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, icc) {
		t.Fatalf("icc=%q want %q", got, icc)
	}
}

func TestMissingProfileDoesNotUseEmbeddedSRGBDefault(t *testing.T) {
	source, media := testProfiles()
	_, err := Proof(Request{Media: media, Intents: AllProofIntents(), Samples: []Sample{{RGB: []float64{0, 0, 0}}}})
	if err == nil {
		t.Fatal("empty source must fail even though tests have an explicit sRGB-like descriptor helper")
	}
	_ = source
}
