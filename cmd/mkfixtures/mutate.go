package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"alphacheck/ebml"
)

func writeVariants(input, outDir string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	alphaZero, err := patchAlphaMode(data, 0)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "mux-alpha-mode-zero.webm"), alphaZero, 0o644); err != nil {
		return err
	}

	timingGap, err := shiftLaterBlockTimecodes(data, 2, 250)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "mux-pts-gap.webm"), timingGap, 0o644); err != nil {
		return err
	}

	cropOK, err := insertCrop(data, 8, 8, 0, 0)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "mux-crop-valid.webm"), cropOK, 0o644); err != nil {
		return err
	}

	cropBad, err := insertCrop(data, 60, 40, 0, 0)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "mux-crop-invalid.webm"), cropBad, 0o644); err != nil {
		return err
	}

	corrupt, err := corruptFirstAlpha(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "encode-corrupt-alpha-header.webm"), corrupt, 0o644); err != nil {
		return err
	}
	return nil
}

func patchAlphaMode(data []byte, value byte) ([]byte, error) {
	out := append([]byte(nil), data...)
	el, err := firstElement(data, ebml.IDAlphaMode)
	if err != nil {
		return nil, err
	}
	if el.Size != 1 {
		return nil, fmt.Errorf("AlphaMode size=%d, want 1", el.Size)
	}
	out[el.Offset+el.HeaderSize] = value
	return out, nil
}

func corruptFirstAlpha(data []byte) ([]byte, error) {
	out := append([]byte(nil), data...)
	el, err := firstElement(data, ebml.IDBlockAdditional)
	if err != nil {
		return nil, err
	}
	if el.Size < 4 {
		return nil, fmt.Errorf("alpha payload too short")
	}
	out[el.Offset+el.HeaderSize+1] ^= 0xff
	return out, nil
}

func firstElement(data []byte, id ebml.ID) (*ebml.Element, error) {
	els, err := ebml.Parse(data)
	if err != nil {
		return nil, err
	}
	var found *ebml.Element
	for _, root := range els {
		root.Walk(func(el *ebml.Element) bool {
			if found == nil && el.ID == id {
				found = el
			}
			return true
		})
		if root.ID == id && found == nil {
			found = root
		}
	}
	if found == nil {
		return nil, fmt.Errorf("element %x not found", id)
	}
	return found, nil
}

func shiftLaterBlockTimecodes(data []byte, startIndex int, deltaMS int16) ([]byte, error) {
	out := append([]byte(nil), data...)
	els, err := ebml.Parse(data)
	if err != nil {
		return nil, err
	}
	index := 0
	durationEl, _ := firstElement(data, ebml.IDDuration)
	for _, root := range els {
		root.Walk(func(el *ebml.Element) bool {
			if el.ID != ebml.IDBlock && el.ID != ebml.IDSimpleBlock {
				return true
			}
			current := index
			index++
			if current < startIndex {
				return true
			}
			idN := vintWidth(el.Data[0])
			off := el.Offset + el.HeaderSize + idN
			v := int16(binary.BigEndian.Uint16(out[off : off+2]))
			binary.BigEndian.PutUint16(out[off:off+2], uint16(v+deltaMS))
			return true
		})
	}
	if durationEl != nil && durationEl.Size == 8 {
		off := durationEl.Offset + durationEl.HeaderSize
		bits := binary.BigEndian.Uint64(out[off : off+8])
		duration := math.Float64frombits(bits) + float64(deltaMS)
		binary.BigEndian.PutUint64(out[off:off+8], math.Float64bits(duration))
	}
	return out, nil
}

func vintWidth(first byte) int {
	for n := 1; n <= 8; n++ {
		if first&(0x80>>uint(n-1)) != 0 {
			return n
		}
	}
	return 0
}
