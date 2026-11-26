package mp4seek

import (
	"encoding/binary"
	"fmt"
)

func rebuildSTBL(file *File, patch *stssPatch, chunkDelta int64) ([]byte, error) {
	data := file.data
	children, err := parseBoxes(data, patch.track.stbl.payload, patch.track.stbl.end)
	if err != nil {
		return nil, err
	}
	stssCount := 0
	insertAfter := -1
	for i, child := range children {
		switch child.typ {
		case "stss":
			stssCount++
		case "stco", "co64":
			insertAfter = i
		}
	}
	if stssCount > 1 {
		return nil, fmt.Errorf("track %d: multiple stss boxes", patch.track.ID)
	}
	if insertAfter < 0 {
		for i, child := range children {
			if child.typ == "stsz" || child.typ == "stsc" || child.typ == "stts" {
				insertAfter = i
			}
		}
	}

	var payload []byte
	pos := patch.track.stbl.payload
	inserted := false
	for i, child := range children {
		payload = append(payload, data[pos:child.start]...)
		switch child.typ {
		case "stss":
			if len(patch.newStss) > 0 {
				payload = append(payload, patch.newStss...)
			}
			inserted = true
		case "stco", "co64":
			replacement, err := patchChunkOffsetBox(data, child, file.moov, chunkDelta)
			if err != nil {
				return nil, fmt.Errorf("track %d: %w", patch.track.ID, err)
			}
			payload = append(payload, replacement...)
		default:
			payload = append(payload, data[child.start:child.end]...)
		}
		if !inserted && stssCount == 0 && len(patch.newStss) > 0 && i == insertAfter {
			payload = append(payload, patch.newStss...)
			inserted = true
		}
		pos = child.end
	}
	payload = append(payload, data[pos:patch.track.stbl.end]...)
	if !inserted && len(patch.newStss) > 0 {
		payload = append(payload, patch.newStss...)
	}
	return makeBoxLike("stbl", patch.track.stbl, data, payload), nil
}

func patchChunkOffsetBox(data []byte, b box, moov box, delta int64) ([]byte, error) {
	if delta == 0 {
		return append([]byte(nil), data[b.start:b.end]...), nil
	}
	if err := needBytes(data, b.payload, 8, b.typ+" header"); err != nil {
		return nil, err
	}
	count := binary.BigEndian.Uint32(data[b.payload+4 : b.payload+8])
	width := 4
	if b.typ == "co64" {
		width = 8
	}
	if err := needBytes(data, b.payload+8, int(count)*width, b.typ+" entries"); err != nil {
		return nil, err
	}
	payload := append([]byte(nil), data[b.payload:b.end]...)
	for i := uint32(0); i < count; i++ {
		off := 8 + int(i)*width
		var old uint64
		if width == 8 {
			old = binary.BigEndian.Uint64(payload[off : off+8])
		} else {
			old = uint64(binary.BigEndian.Uint32(payload[off : off+4]))
		}
		shifted, err := shiftChunkOffset(old, moov, delta)
		if err != nil {
			return nil, fmt.Errorf("%s entry %d: %w", b.typ, i+1, err)
		}
		if width == 8 {
			binary.BigEndian.PutUint64(payload[off:off+8], shifted)
		} else {
			if shifted > uint64(^uint32(0)) {
				return nil, fmt.Errorf("stco entry %d overflows 32 bits after repair; co64 required", i+1)
			}
			binary.BigEndian.PutUint32(payload[off:off+4], uint32(shifted))
		}
	}
	return makeBoxLike(b.typ, b, data, payload), nil
}

func shiftChunkOffset(offset uint64, moov box, delta int64) (uint64, error) {
	if delta == 0 {
		return offset, nil
	}
	if offset >= uint64(moov.end) {
		shifted := int64(offset) + delta
		if shifted < 0 {
			return 0, fmt.Errorf("chunk offset %d cannot be shifted by %d", offset, delta)
		}
		return uint64(shifted), nil
	}
	if offset >= uint64(moov.start) {
		return 0, fmt.Errorf("chunk offset %d points inside moov", offset)
	}
	return offset, nil
}
