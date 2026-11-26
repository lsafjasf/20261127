package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Box struct {
	Type       string
	Start      int64
	HeaderSize int64
	Size       int64
	Payload    []byte
}

func (b Box) End() int64 {
	return b.Start + b.Size
}

func readTopLevel(r io.ReaderAt, fileSize int64) ([]Box, error) {
	var boxes []Box
	for off := int64(0); off < fileSize; {
		b, err := readBoxHeader(r, off, fileSize)
		if err != nil {
			return boxes, err
		}
		if b.Size < b.HeaderSize || b.End() > fileSize {
			return boxes, fmt.Errorf("box %q at %d has invalid size %d", b.Type, off, b.Size)
		}
		boxes = append(boxes, b)
		if b.Size == 0 {
			break
		}
		off = b.End()
	}
	return boxes, nil
}

func readBoxHeader(r io.ReaderAt, off, fileSize int64) (Box, error) {
	var hdr [16]byte
	n, err := r.ReadAt(hdr[:8], off)
	if err != nil && err != io.EOF {
		return Box{}, err
	}
	if n < 8 {
		return Box{}, fmt.Errorf("truncated box header at %d", off)
	}
	size := int64(binary.BigEndian.Uint32(hdr[:4]))
	typ := string(hdr[4:8])
	headerSize := int64(8)
	if size == 1 {
		n, err = r.ReadAt(hdr[8:16], off+8)
		if err != nil && err != io.EOF {
			return Box{}, err
		}
		if n < 8 {
			return Box{}, fmt.Errorf("truncated large box header at %d", off)
		}
		size = int64(binary.BigEndian.Uint64(hdr[8:16]))
		headerSize = 16
	} else if size == 0 {
		size = fileSize - off
	}
	if typ == "uuid" {
		headerSize += 16
	}
	return Box{Type: typ, Start: off, HeaderSize: headerSize, Size: size}, nil
}

func readPayload(r io.ReaderAt, b Box) ([]byte, error) {
	payloadSize := b.Size - b.HeaderSize
	if payloadSize < 0 || payloadSize > 256<<20 {
		return nil, fmt.Errorf("box %q payload size %d is unsupported", b.Type, payloadSize)
	}
	data := make([]byte, payloadSize)
	if payloadSize == 0 {
		return data, nil
	}
	if _, err := r.ReadAt(data, b.Start+b.HeaderSize); err != nil {
		return nil, fmt.Errorf("read box %q: %w", b.Type, err)
	}
	return data, nil
}

func ParseChildren(data []byte) ([]Box, error) {
	var out []Box
	for off := 0; off < len(data); {
		if len(data)-off < 8 {
			return out, fmt.Errorf("%d trailing bytes after child boxes", len(data)-off)
		}
		size := int64(binary.BigEndian.Uint32(data[off : off+4]))
		typ := string(data[off+4 : off+8])
		headerSize := int64(8)
		if size == 1 {
			if len(data)-off < 16 {
				return out, fmt.Errorf("truncated large child box %q", typ)
			}
			size = int64(binary.BigEndian.Uint64(data[off+8 : off+16]))
			headerSize = 16
		} else if size == 0 {
			size = int64(len(data) - off)
		}
		if typ == "uuid" {
			headerSize += 16
		}
		if size < headerSize || int64(off)+size > int64(len(data)) {
			return out, fmt.Errorf("child box %q at %d has invalid size %d", typ, off, size)
		}
		out = append(out, Box{
			Type:       typ,
			Start:      int64(off),
			HeaderSize: headerSize,
			Size:       size,
			Payload:    data[off+int(headerSize) : off+int(size)],
		})
		off += int(size)
	}
	return out, nil
}

func Find(boxes []Box, typ string) *Box {
	for i := range boxes {
		if boxes[i].Type == typ {
			return &boxes[i]
		}
	}
	return nil
}

func FindAll(boxes []Box, typ string) []Box {
	var out []Box
	for _, b := range boxes {
		if b.Type == typ {
			out = append(out, b)
		}
	}
	return out
}
