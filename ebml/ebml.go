// Package ebml implements the small canonical EBML subset used by Matroska
// metadata. It has no external muxer dependency.
package ebml

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

type ID uint32

var (
	ErrTruncated = errors.New("ebml: truncated element")
	ErrVINT      = errors.New("ebml: invalid variable-length integer")
)

type Header struct {
	ID           ID
	IDLength     int
	SizeLength   int
	HeaderLength int
	Size         uint64
	UnknownSize  bool
}

func Width(first byte) int {
	for n := 1; n <= 8; n++ {
		if first&(0x80>>uint(n-1)) != 0 {
			return n
		}
	}
	return 0
}

func ReadHeader(r io.Reader) (Header, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return Header{}, err
	}
	idLen := Width(first[0])
	if idLen < 1 || idLen > 4 {
		return Header{}, fmt.Errorf("%w: id width from byte %02x", ErrVINT, first[0])
	}
	idBytes := make([]byte, idLen)
	idBytes[0] = first[0]
	if _, err := io.ReadFull(r, idBytes[1:]); err != nil {
		return Header{}, fmt.Errorf("%w: element id", ErrTruncated)
	}
	id := ID(binary.BigEndian.Uint32(append(make([]byte, 4-idLen), idBytes...)))

	if _, err := io.ReadFull(r, first[:]); err != nil {
		return Header{}, fmt.Errorf("%w: size first byte", ErrTruncated)
	}
	sizeLen := Width(first[0])
	if sizeLen < 1 || sizeLen > 8 {
		return Header{}, fmt.Errorf("%w: size width from byte %02x", ErrVINT, first[0])
	}
	sizeBytes := make([]byte, sizeLen)
	sizeBytes[0] = first[0]
	if _, err := io.ReadFull(r, sizeBytes[1:]); err != nil {
		return Header{}, fmt.Errorf("%w: element size", ErrTruncated)
	}
	value := uint64(sizeBytes[0] & (0xff >> uint(sizeLen)))
	for i := 1; i < sizeLen; i++ {
		value = value<<8 | uint64(sizeBytes[i])
	}
	allBits := uint64(1)<<uint(7*sizeLen) - 1
	return Header{
		ID: id, IDLength: idLen, SizeLength: sizeLen, HeaderLength: idLen + sizeLen,
		Size: value, UnknownSize: value == allBits,
	}, nil
}

func EncodeID(id ID) []byte {
	raw := make([]byte, 4)
	binary.BigEndian.PutUint32(raw, uint32(id))
	for len(raw) > 1 && raw[0] == 0 {
		raw = raw[1:]
	}
	return raw
}

func EncodeSize(size uint64) []byte {
	for n := 1; n <= 8; n++ {
		maxValid := uint64(1)<<uint(7*n) - 2
		if size <= maxValid {
			out := make([]byte, n)
			for i := n - 1; i >= 0; i-- {
				out[i] = byte(size & 0xff)
				size >>= 8
			}
			out[0] |= 0x80 >> uint(n-1)
			return out
		}
	}
	panic("ebml: size too large")
}

func EncodeUnknownSize(width int) []byte {
	if width < 1 || width > 8 {
		width = 8
	}
	out := bytes.Repeat([]byte{0xff}, width)
	out[0] = 0xff >> uint(width-1)
	out[0] |= 0x80 >> uint(width-1)
	return out
}

func ElementBytes(id ID, payload []byte) []byte {
	var buf bytes.Buffer
	buf.Write(EncodeID(id))
	buf.Write(EncodeSize(uint64(len(payload))))
	buf.Write(payload)
	return buf.Bytes()
}

func ReadUint(data []byte) uint64 {
	var value uint64
	for _, b := range data {
		value = value<<8 | uint64(b)
	}
	return value
}

func EncodeUint(value uint64) []byte {
	if value == 0 {
		return []byte{0}
	}
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	start := 0
	for start < 7 && raw[start] == 0 {
		start++
	}
	return raw[start:]
}

func EncodeString(value string) []byte { return []byte(value) }

func ReadFloat(data []byte) (float64, error) {
	switch len(data) {
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(data))), nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(data)), nil
	case 0:
		return 0, nil
	default:
		return 0, fmt.Errorf("ebml: unsupported float length %d", len(data))
	}
}
