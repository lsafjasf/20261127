package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

func fullBoxVersion(r io.ReaderAt, b *Box) (byte, error) {
	var v [1]byte
	if _, err := r.ReadAt(v[:], b.Payload()); err != nil {
		return 0, fmt.Errorf("read %s full-box version: %w", b.Type, err)
	}
	return v[0], nil
}

func readU16(r io.ReaderAt, off int64) (uint16, error) {
	var b [2]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b[:]), nil
}

func readU32(r io.ReaderAt, off int64) (uint32, error) {
	var b [4]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b[:]), nil
}

func readU64(r io.ReaderAt, off int64) (uint64, error) {
	var b [8]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b[:]), nil
}

func readI32(r io.ReaderAt, off int64) (int32, error) {
	var b [4]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b[:])), nil
}

func readI64(r io.ReaderAt, off int64) (int64, error) {
	var b [8]byte
	if _, err := r.ReadAt(b[:], off); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b[:])), nil
}

func payloadLimit(b *Box, need int64) error {
	if b == nil {
		return fmt.Errorf("box is missing")
	}
	if b.End()-b.Payload() < need {
		return fmt.Errorf("%s payload is truncated", b.Type)
	}
	return nil
}
