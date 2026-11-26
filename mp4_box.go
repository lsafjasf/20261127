package laddercheck

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Box is a located ISO-BMFF box; payload bytes are read on demand so that
// large mdat ranges are never pulled into memory.
type Box struct {
	Type     string
	Start    int64
	HSize    int64
	Payload  int64
	End      int64
	Children []*Box
}

func (b *Box) child(typ string) *Box {
	for _, c := range b.Children {
		if c.Type == typ {
			return c
		}
	}
	return nil
}

func (b *Box) childrenOf(typ string) []*Box {
	var out []*Box
	for _, c := range b.Children {
		if c.Type == typ {
			out = append(out, c)
		}
	}
	return out
}

// path resolves a descendant chain such as "mdia/mdhd".
func (b *Box) path(types ...string) *Box {
	cur := b
	for _, t := range types {
		cur = cur.child(t)
		if cur == nil {
			return nil
		}
	}
	return cur
}

type readerAt struct{ ra io.ReaderAt }

func (r *readerAt) bytes(off, n int64) ([]byte, error) {
	buf := make([]byte, n)
	_, err := io.ReadFull(io.NewSectionReader(r.ra, off, n), buf)
	return buf, err
}

func (r *readerAt) u8(off int64) (byte, error) {
	b, err := r.bytes(off, 1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *readerAt) u16(off int64) (uint16, error) {
	b, err := r.bytes(off, 2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *readerAt) u24(off int64) (uint32, error) {
	b, err := r.bytes(off, 3)
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2]), nil
}

func (r *readerAt) u32(off int64) (uint32, error) {
	b, err := r.bytes(off, 4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (r *readerAt) u64(off int64) (uint64, error) {
	b, err := r.bytes(off, 8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}

// containerBoxes are the box types whose payload is itself a box hierarchy.
var containerBoxes = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"stbl": true, "edts": true, "dinf": true, "mvex": true,
	"moof": true, "traf": true, "udta": false,
}

func buildTree(r *readerAt, start, end int64, depth int) ([]*Box, error) {
	if depth > 16 {
		return nil, fmt.Errorf("laddercheck: box nesting too deep")
	}
	var boxes []*Box
	off := start
	for off+8 <= end {
		size32, err := r.u32(off)
		if err != nil {
			return boxes, err
		}
		typBytes, err := r.bytes(off+4, 4)
		if err != nil {
			return boxes, err
		}
		typ := string(typBytes)
		hsize := int64(8)
		size := int64(size32)
		switch size32 {
		case 1:
			full, err := r.u64(off + 8)
			if err != nil {
				return boxes, err
			}
			size = int64(full)
			hsize = 16
		case 0:
			size = end - off
		}
		if size < hsize {
			return boxes, fmt.Errorf("laddercheck: malformed box %q at %d (size %d)", typ, off, size)
		}
		boxEnd := off + size
		if boxEnd > end {
			// Tolerate a trailing overrun: clamp to the enclosing range.
			boxEnd = end
		}
		bx := &Box{Type: typ, Start: off, HSize: hsize, Payload: off + hsize, End: boxEnd}
		if containerBoxes[typ] && bx.Payload < boxEnd {
			children, err := buildTree(r, bx.Payload, boxEnd, depth+1)
			if err != nil {
				return boxes, err
			}
			bx.Children = children
		}
		boxes = append(boxes, bx)
		off = boxEnd
	}
	return boxes, nil
}

// ParseMP4File parses one ISO-BMFF/MOV file into normalized facts.
func ParseMP4File(path string) (*RenditionFacts, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	facts, err := ParseMP4(f, st.Size())
	if err != nil {
		return nil, err
	}
	facts.Path = path
	if facts.ID == "" {
		facts.ID = path
	}
	return facts, nil
}

// ParseMP4 parses from any ReaderAt with a known size.
func ParseMP4(ra io.ReaderAt, size int64) (*RenditionFacts, error) {
	rd := &readerAt{ra: ra}
	if looksLikeMPEGTS(rd, size) {
		return nil, fmt.Errorf("laddercheck: MPEG-TS cannot be box-parsed; export GOP/IDR/first-PTS evidence to JSON and pass via -in (see examples/ladder.evidence.json)")
	}
	root, err := buildTree(rd, 0, size, 0)
	if err != nil {
		return nil, err
	}
	return factsFromTree(rd, root, size)
}

func looksLikeMPEGTS(r *readerAt, size int64) bool {
	b0, err := r.u8(0)
	if err != nil || b0 != 0x47 {
		return false
	}
	if size < 189 {
		return size >= 188
	}
	if b, err := r.u8(188); err == nil && b == 0x47 {
		return true
	}
	if b, err := r.u8(192); err == nil && b == 0x47 { // some 192-byte packetings
		return true
	}
	return false
}
