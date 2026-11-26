package iccproof

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadEmbeddedICC extracts an ICC profile from a PNG iCCP chunk or JPEG APP2
// ICC_PROFILE segments. It returns nil when no embedded ICC is present.
func ReadEmbeddedICC(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return readPNGICC(data)
	case ".jpg", ".jpeg":
		return readJPEGICC(data)
	default:
		// Sniff content for callers with unusual extensions.
		if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
			return readPNGICC(data)
		}
		if len(data) >= 2 && data[0] == 0xff && data[1] == 0xd8 {
			return readJPEGICC(data)
		}
		return nil, nil
	}
}

func readPNGICC(data []byte) ([]byte, error) {
	if len(data) < 8 || !bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return nil, fmt.Errorf("not a PNG file")
	}
	off := 8
	for off+12 <= len(data) {
		length := binary.BigEndian.Uint32(data[off : off+4])
		chunkType := string(data[off+4 : off+8])
		end := off + 12 + int(length)
		if end > len(data) {
			return nil, fmt.Errorf("truncated PNG %s chunk", chunkType)
		}
		if chunkType == "iCCP" {
			payload := data[off+8 : off+8+int(length)]
			nul := bytes.IndexByte(payload, 0)
			if nul < 0 || nul+2 >= len(payload) {
				return nil, fmt.Errorf("invalid PNG iCCP chunk")
			}
			compressionMethod := payload[nul+1]
			if compressionMethod != 0 {
				return nil, fmt.Errorf("unsupported iCCP compression method %d", compressionMethod)
			}
			zr, err := zlib.NewReader(bytes.NewReader(payload[nul+2:]))
			if err != nil {
				return nil, fmt.Errorf("open iCCP zlib: %w", err)
			}
			icc, err := io.ReadAll(zr)
			_ = zr.Close()
			if err != nil {
				return nil, fmt.Errorf("decompress iCCP: %w", err)
			}
			return icc, nil
		}
		if chunkType == "IDAT" {
			break
		}
		off = end
	}
	return nil, nil
}

func readJPEGICC(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
		return nil, fmt.Errorf("not a JPEG file")
	}
	chunks := map[int][]byte{}
	total := 0
	off := 2
	for off+4 < len(data) {
		if data[off] != 0xff {
			off++
			continue
		}
		marker := data[off+1]
		if marker == 0xda { // SOS: image scan follows, no more metadata markers.
			break
		}
		if marker == 0xd8 || (marker >= 0xd0 && marker <= 0xd9) {
			off += 2
			continue
		}
		segLen := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		if segLen < 2 || off+2+segLen > len(data) {
			return nil, fmt.Errorf("invalid JPEG marker length at byte %d", off)
		}
		seg := data[off+4 : off+2+segLen]
		if marker == 0xe2 && len(seg) >= 14 && bytes.Equal(seg[:12], []byte("ICC_PROFILE\x00")) {
			seq := int(seg[12])
			total = int(seg[13])
			if seq < 1 || total < 1 || seq > total {
				return nil, fmt.Errorf("invalid JPEG ICC segment number %d/%d", seq, total)
			}
			chunks[seq] = append([]byte(nil), seg[14:]...)
		}
		off += 2 + segLen
	}
	if total == 0 {
		return nil, nil
	}
	var icc []byte
	for i := 1; i <= total; i++ {
		chunk, ok := chunks[i]
		if !ok {
			return nil, fmt.Errorf("missing JPEG ICC segment %d/%d", i, total)
		}
		icc = append(icc, chunk...)
	}
	return icc, nil
}

// SourceProfileFromPhoto returns an embedded profile reference when present.
// It returns an empty reference and nil error when the photo has no ICC.
func SourceProfileFromPhoto(path string) (ProfileRef, error) {
	icc, err := ReadEmbeddedICC(path)
	if err != nil {
		return ProfileRef{}, err
	}
	if len(icc) == 0 {
		return ProfileRef{}, nil
	}
	return ProfileRef{Path: path + "#embedded-icc", Bytes: icc}, nil
}
