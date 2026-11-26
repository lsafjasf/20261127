package main

import (
	"fmt"

	"alphacheck/ebml"
)

func insertCrop(data []byte, left, right, top, bottom uint64) ([]byte, error) {
	els, err := ebml.Parse(data)
	if err != nil {
		return nil, err
	}
	var segment, tracks, entry, video *ebml.Element
	for _, root := range els {
		if root.ID != ebml.IDSegment {
			continue
		}
		segment = root
		tracks = root.Find(ebml.IDTracks)
		if tracks != nil {
			for _, te := range tracks.FindAll(ebml.IDTrackEntry) {
				if v := te.Find(ebml.IDVideo); v != nil {
					entry, video = te, v
					break
				}
			}
		}
	}
	if segment == nil || tracks == nil || entry == nil || video == nil {
		return nil, fmt.Errorf("video TrackEntry not found")
	}

	patch := make([]byte, 0, 16)
	patch = appendUintElement(patch, uint32(ebml.IDCropLeft), left)
	patch = appendUintElement(patch, uint32(ebml.IDCropRight), right)
	patch = appendUintElement(patch, uint32(ebml.IDCropTop), top)
	patch = appendUintElement(patch, uint32(ebml.IDCropBottom), bottom)

	insertAt := video.Offset + video.HeaderSize + int(video.Size)
	out := append([]byte(nil), data[:insertAt]...)
	out = append(out, patch...)
	out = append(out, data[insertAt:]...)
	delta := uint64(len(patch))

	for _, el := range []*ebml.Element{video, entry, tracks, segment} {
		if err := rewriteSizeVINT(out, el, el.Size+delta); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func appendUintElement(out []byte, id uint32, value uint64) []byte {
	if value == 0 {
		return out
	}
	idBytes := []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	if id&0xffff0000 == 0 {
		idBytes = []byte{byte(id >> 8), byte(id)}
	}
	out = append(out, idBytes...)
	size := minimalUintBytes(value)
	out = append(out, 0x80|byte(size))
	for i := size - 1; i >= 0; i-- {
		out = append(out, byte(value>>uint(8*i)))
	}
	return out
}

func minimalUintBytes(v uint64) int {
	switch {
	case v <= 0xff:
		return 1
	case v <= 0xffff:
		return 2
	case v <= 0xffffffff:
		return 4
	default:
		return 8
	}
}

func rewriteSizeVINT(out []byte, el *ebml.Element, newSize uint64) error {
	idN := vintWidth(out[el.Offset])
	sizeOff := el.Offset + idN
	sizeN := vintWidth(out[sizeOff])
	if sizeN < 1 || sizeN > 8 {
		return fmt.Errorf("invalid existing size VINT at %d", sizeOff)
	}
	maxSize := uint64(1)<<uint(7*sizeN) - 1
	if newSize > maxSize {
		return fmt.Errorf("new size %d exceeds %d-byte VINT", newSize, sizeN)
	}
	marker := byte(0x80 >> uint(sizeN-1))
	vint := make([]byte, sizeN)
	v := newSize
	for i := sizeN - 1; i >= 1; i-- {
		vint[i] = byte(v)
		v >>= 8
	}
	vint[0] = marker | byte(v)
	copy(out[sizeOff:sizeOff+sizeN], vint)
	return nil
}
