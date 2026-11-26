package mediameta

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxMOOVBytes = 256 << 20
const mdatPrefixBytes = 64 << 10

type atomHeader struct {
	size       int64
	headerSize int64
	atomType   string
}

func parseMOVFile(path string) (*MOVInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	end := stat.Size()
	info := &MOVInfo{}

	var offset int64
	for offset+8 <= end {
		header, err := readAtomHeader(file, offset, end)
		if err != nil {
			return nil, fmt.Errorf("read atom at %d: %w", offset, err)
		}
		if header.size < header.headerSize || offset+header.size > end {
			break
		}

		switch header.atomType {
		case "ftyp":
			major := make([]byte, 4)
			if _, err := file.ReadAt(major, offset+header.headerSize); err == nil {
				info.MajorBrand = string(major)
			}
		case "moov":
			if err := readColorBox(file, offset, header, info); err != nil {
				return nil, err
			}
		case "mdat":
			if info.ICPFCount == 0 {
				readProResMarkers(file, offset, header, info)
			}
		}

		offset += header.size
	}

	if info.ICPFCount == 0 {
		scanProResInPrefix(file, 0, minInt64(end, 1<<20), info)
	}
	info.ProResAlphaMarked = len(info.ProResAlphaTypes) > 0 && info.ProResAlphaTypes[0] != 0
	return info, nil
}

func readAtomHeader(r io.ReaderAt, offset, fileSize int64) (atomHeader, error) {
	headerBytes := make([]byte, 16)
	if _, err := r.ReadAt(headerBytes[:8], offset); err != nil {
		return atomHeader{}, err
	}
	size := int64(binary.BigEndian.Uint32(headerBytes[0:4]))
	atomType := string(headerBytes[4:8])
	headerSize := int64(8)
	if size == 1 {
		if _, err := r.ReadAt(headerBytes[8:16], offset+8); err != nil {
			return atomHeader{}, err
		}
		size = int64(binary.BigEndian.Uint64(headerBytes[8:16]))
		headerSize = 16
	} else if size == 0 {
		size = fileSize - offset
	}
	return atomHeader{size: size, headerSize: headerSize, atomType: atomType}, nil
}

func readColorBox(file *os.File, offset int64, header atomHeader, info *MOVInfo) error {
	payloadSize := header.size - header.headerSize
	if payloadSize <= 0 || payloadSize > maxMOOVBytes {
		return nil
	}
	data := make([]byte, payloadSize)
	if _, err := file.ReadAt(data, offset+header.headerSize); err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	idx := indexFourCC(data, "colr")
	if idx < 0 || idx+14 > len(data) {
		return nil
	}
	box := &ColorBox{
		Offset:    offset + header.headerSize + int64(idx) - 4,
		Size:      int64(binary.BigEndian.Uint32(data[idx-4 : idx])),
		ColorType: string(data[idx+4 : idx+8]),
		Primaries: binary.BigEndian.Uint16(data[idx+8 : idx+10]),
		Transfer:  binary.BigEndian.Uint16(data[idx+10 : idx+12]),
		Matrix:    binary.BigEndian.Uint16(data[idx+12 : idx+14]),
	}
	if box.ColorType == "nclx" && idx+15 <= len(data) {
		flag := data[idx+14]&0x80 != 0
		box.FullRangeFlag = &flag
	}
	box.PrimariesName = primariesName(box.Primaries)
	box.TransferName = transferName(box.Transfer)
	box.MatrixName = matrixName(box.Matrix)
	info.ColorBox = box
	return nil
}

func readProResMarkers(file *os.File, offset int64, header atomHeader, info *MOVInfo) {
	size := minInt64(header.size-header.headerSize, mdatPrefixBytes)
	if size <= 32 {
		return
	}
	data := make([]byte, size)
	if _, err := file.ReadAt(data, offset+header.headerSize); err != nil && !errors.Is(err, io.EOF) {
		return
	}
	scanProResBytes(data, info)
}

func scanProResInPrefix(file *os.File, offset, size int64, info *MOVInfo) {
	if size <= 32 {
		return
	}
	data := make([]byte, size)
	if _, err := file.ReadAt(data, offset); err != nil && !errors.Is(err, io.EOF) {
		return
	}
	scanProResBytes(data, info)
}

func scanProResBytes(data []byte, info *MOVInfo) {
	start := 0
	for {
		idx := indexFourCC(data[start:], "icpf")
		if idx < 0 {
			return
		}
		absolute := start + idx
		info.ICPFCount++
		if absolute+22 < len(data) {
			alphaType := int(data[absolute+4+17] & 0x0f)
			info.ProResAlphaTypes = append(info.ProResAlphaTypes, alphaType)
		}
		start = absolute + 4
		if info.ICPFCount >= 4 {
			return
		}
	}
}

func indexFourCC(data []byte, code string) int {
	if len(code) != 4 {
		return -1
	}
	target := []byte(code)
	for i := 0; i+4 <= len(data); i++ {
		if data[i] == target[0] && data[i+1] == target[1] && data[i+2] == target[2] && data[i+3] == target[3] {
			return i
		}
	}
	return -1
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
