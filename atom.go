package mp4seek

import (
	"encoding/binary"
	"fmt"
)

const (
	boxHeaderSize         = 8
	extendedBoxHeaderSize = 16
)

type box struct {
	typ         string
	start       int
	end         int
	header      int
	payload     int
	userTypeOff int
}

func (b box) size() int {
	return b.end - b.start
}

func (b box) payloadSize() int {
	return b.end - b.payload
}

func parseBoxes(data []byte, start, end int) ([]box, error) {
	if start < 0 || end < start || end > len(data) {
		return nil, fmt.Errorf("invalid box range [%d,%d) for %d-byte file", start, end, len(data))
	}
	var boxes []box
	for pos := start; pos < end; {
		if end-pos < boxHeaderSize {
			if allZero(data[pos:end]) {
				break
			}
			return nil, fmt.Errorf("truncated box header at offset %d", pos)
		}
		b, err := parseBoxAt(data, pos, end)
		if err != nil {
			return nil, err
		}
		boxes = append(boxes, b)
		pos = b.end
	}
	return boxes, nil
}

func parseBoxAt(data []byte, pos, parentEnd int) (box, error) {
	if parentEnd-pos < boxHeaderSize {
		return box{}, fmt.Errorf("truncated box header at offset %d", pos)
	}
	size32 := binary.BigEndian.Uint32(data[pos : pos+4])
	typ := string(data[pos+4 : pos+8])
	header := boxHeaderSize
	size := uint64(size32)
	switch size32 {
	case 0:
		size = uint64(parentEnd - pos)
	case 1:
		if parentEnd-pos < extendedBoxHeaderSize {
			return box{}, fmt.Errorf("truncated 64-bit size for %q box at offset %d", typ, pos)
		}
		size = binary.BigEndian.Uint64(data[pos+8 : pos+16])
		header = extendedBoxHeaderSize
	}
	userTypeOff := 0
	if typ == "uuid" {
		userTypeOff = header
		header += 16
	}
	if size < uint64(header) {
		return box{}, fmt.Errorf("box %q at offset %d has size %d smaller than its header", typ, pos, size)
	}
	if size > uint64(parentEnd-pos) {
		return box{}, fmt.Errorf("box %q at offset %d extends past parent by %d bytes", typ, pos, size-uint64(parentEnd-pos))
	}
	return box{
		typ:         typ,
		start:       pos,
		end:         pos + int(size),
		header:      header,
		payload:     pos + header,
		userTypeOff: userTypeOff,
	}, nil
}

func findBox(data []byte, parent box, typ string) (box, bool, error) {
	children, err := parseBoxes(data, parent.payload, parent.end)
	if err != nil {
		return box{}, false, err
	}
	for _, child := range children {
		if child.typ == typ {
			return child, true, nil
		}
	}
	return box{}, false, nil
}

func findBoxes(data []byte, parent box, typ string) ([]box, error) {
	children, err := parseBoxes(data, parent.payload, parent.end)
	if err != nil {
		return nil, err
	}
	var found []box
	for _, child := range children {
		if child.typ == typ {
			found = append(found, child)
		}
	}
	return found, nil
}

func makeBox(typ string, payload []byte) []byte {
	return makeBoxLike(typ, box{}, nil, payload)
}

func makeBoxLike(typ string, old box, oldData, payload []byte) []byte {
	useExtended := old.header >= extendedBoxHeaderSize
	if uint64(len(payload))+boxHeaderSize > uint64(^uint32(0)) {
		useExtended = true
	}
	header := boxHeaderSize
	if useExtended {
		header = extendedBoxHeaderSize
	}
	var userType []byte
	if typ == "uuid" {
		if old.userTypeOff > 0 && old.userTypeOff+16 <= len(oldData) {
			userType = oldData[old.userTypeOff : old.userTypeOff+16]
		} else {
			userType = make([]byte, 16)
		}
		header += len(userType)
	}
	out := make([]byte, header+len(payload))
	total := uint64(len(out))
	if useExtended {
		binary.BigEndian.PutUint32(out[0:4], 1)
		binary.BigEndian.PutUint64(out[8:16], total)
	} else {
		binary.BigEndian.PutUint32(out[0:4], uint32(total))
	}
	copy(out[4:8], typ)
	if typ == "uuid" {
		copy(out[8:8+len(userType)], userType)
	}
	copy(out[header:], payload)
	return out
}

func replaceChild(data []byte, parent, oldChild box, replacement []byte) []byte {
	payload := make([]byte, 0, parent.payloadSize()-oldChild.size()+len(replacement))
	payload = append(payload, data[parent.payload:oldChild.start]...)
	payload = append(payload, replacement...)
	payload = append(payload, data[oldChild.end:parent.end]...)
	return makeBoxLike(parent.typ, parent, data, payload)
}

func allZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

func needBytes(data []byte, off, n int, what string) error {
	if off < 0 || n < 0 || off > len(data)-n {
		return fmt.Errorf("truncated %s at offset %d: need %d bytes", what, off, n)
	}
	return nil
}

func readU32(data []byte, off int, what string) (uint32, error) {
	if err := needBytes(data, off, 4, what); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(data[off : off+4]), nil
}

func readU64(data []byte, off int, what string) (uint64, error) {
	if err := needBytes(data, off, 8, what); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(data[off : off+8]), nil
}

func readI32(data []byte, off int, what string) (int32, error) {
	v, err := readU32(data, off, what)
	return int32(v), err
}
