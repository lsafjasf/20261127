package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type Box struct {
	Type       string
	Start      int64
	Size       int64
	HeaderSize int64
	Children   []*Box
}

func (b *Box) End() int64     { return b.Start + b.Size }
func (b *Box) Payload() int64 { return b.Start + b.HeaderSize }

var containerBoxes = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"stbl": true, "edts": true, "dinf": true, "udta": true,
}

const maxBoxDepth = 24
const maxChildren = 100000

func readBoxes(r io.ReaderAt, start, end int64, depth int) ([]*Box, error) {
	if depth > maxBoxDepth {
		return nil, errors.New("mp4 box nesting is too deep")
	}
	var boxes []*Box
	for pos := start; pos < end; {
		if end-pos < 8 {
			return nil, fmt.Errorf("truncated mp4 box header at offset %d", pos)
		}
		var head [16]byte
		if _, err := r.ReadAt(head[:8], pos); err != nil {
			return nil, fmt.Errorf("read mp4 box at %d: %w", pos, err)
		}
		typ := string(head[4:8])
		size := int64(binary.BigEndian.Uint32(head[:4]))
		headerSize := int64(8)
		if size == 1 {
			if end-pos < 16 {
				return nil, fmt.Errorf("truncated 64-bit box header at offset %d", pos)
			}
			if _, err := r.ReadAt(head[8:16], pos+8); err != nil {
				return nil, fmt.Errorf("read large mp4 box at %d: %w", pos, err)
			}
			size = int64(binary.BigEndian.Uint64(head[8:16]))
			headerSize = 16
		} else if size == 0 {
			size = end - pos
		}
		if size < headerSize || size > end-pos {
			return nil, fmt.Errorf("invalid mp4 box %q at %d: size %d", typ, pos, size)
		}
		b := &Box{Type: typ, Start: pos, Size: size, HeaderSize: headerSize}
		if containerBoxes[typ] {
			children, err := readBoxes(r, pos+headerSize, pos+size, depth+1)
			if err != nil {
				return nil, err
			}
			b.Children = children
		}
		boxes = append(boxes, b)
		if len(boxes) > maxChildren {
			return nil, errors.New("too many mp4 boxes")
		}
		pos += size
	}
	return boxes, nil
}

func child(b *Box, typ string) *Box {
	if b == nil {
		return nil
	}
	for _, c := range b.Children {
		if c.Type == typ {
			return c
		}
	}
	return nil
}

func path(b *Box, parts ...string) *Box {
	cur := b
	for _, p := range parts {
		cur = child(cur, p)
		if cur == nil {
			return nil
		}
	}
	return cur
}
