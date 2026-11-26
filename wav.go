package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	pcmFormat        = 1
	ieeeFloatFormat  = 3
	extensibleFormat = 65534
)

type WAVFile struct {
	SampleRate int
	Left       []float64
	Right      []float64
}

func ReadWAV(reader io.Reader) (WAVFile, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return WAVFile{}, err
	}
	if len(data) < 44 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
		return WAVFile{}, errors.New("not a RIFF/WAVE file")
	}

	var formatCode uint16
	var channels uint16
	var sampleRate uint32
	var bitsPerSample uint16
	var blockAlign uint16
	var payload []byte

	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(data) {
			return WAVFile{}, fmt.Errorf("truncated %q chunk", chunkID)
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return WAVFile{}, errors.New("invalid fmt chunk")
			}
			formatCode = binary.LittleEndian.Uint16(data[chunkStart:])
			channels = binary.LittleEndian.Uint16(data[chunkStart+2:])
			sampleRate = binary.LittleEndian.Uint32(data[chunkStart+4:])
			blockAlign = binary.LittleEndian.Uint16(data[chunkStart+12:])
			bitsPerSample = binary.LittleEndian.Uint16(data[chunkStart+14:])
			if formatCode == extensibleFormat && chunkSize >= 40 {
				formatCode = binary.LittleEndian.Uint16(data[chunkStart+24:])
			}
		case "data":
			payload = data[chunkStart:chunkEnd]
		}

		offset = chunkEnd + chunkSize%2
	}

	if channels != 2 {
		return WAVFile{}, fmt.Errorf("stereo input required, got %d channels", channels)
	}
	if sampleRate == 0 || bitsPerSample == 0 || blockAlign == 0 || payload == nil {
		return WAVFile{}, errors.New("missing required WAV fmt or data chunk")
	}
	if formatCode != pcmFormat && formatCode != ieeeFloatFormat {
		return WAVFile{}, fmt.Errorf("unsupported audio format %d; supported formats are PCM and IEEE float", formatCode)
	}

	bytesPerSample := int(bitsPerSample) / 8
	if bytesPerSample == 0 || int(blockAlign) < int(channels)*bytesPerSample {
		return WAVFile{}, errors.New("invalid WAV sample layout")
	}

	frameCount := len(payload) / int(blockAlign)
	left := make([]float64, frameCount)
	right := make([]float64, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		base := frame * int(blockAlign)
		left[frame], err = decodeSample(payload[base:], formatCode, bytesPerSample)
		if err != nil {
			return WAVFile{}, err
		}
		right[frame], err = decodeSample(payload[base+bytesPerSample:], formatCode, bytesPerSample)
		if err != nil {
			return WAVFile{}, err
		}
	}

	return WAVFile{
		SampleRate: int(sampleRate),
		Left:       left,
		Right:      right,
	}, nil
}

func decodeSample(data []byte, format uint16, bytesPerSample int) (float64, error) {
	if len(data) < bytesPerSample {
		return 0, errors.New("truncated audio sample")
	}
	switch format {
	case pcmFormat:
		return decodePCM(data, bytesPerSample)
	case ieeeFloatFormat:
		if bytesPerSample != 4 {
			return 0, fmt.Errorf("unsupported float bit depth: %d", bytesPerSample*8)
		}
		bits := binary.LittleEndian.Uint32(data[:4])
		return float64(math.Float32frombits(bits)), nil
	default:
		return 0, errors.New("unsupported audio format")
	}
}

func decodePCM(data []byte, bytesPerSample int) (float64, error) {
	switch bytesPerSample {
	case 1:
		return (float64(data[0]) - 128) / 128, nil
	case 2:
		return float64(int16(binary.LittleEndian.Uint16(data[:2]))) / 32768, nil
	case 3:
		value := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
		if value&0x800000 != 0 {
			value |= ^0xffffff
		}
		return float64(value) / 8388608, nil
	case 4:
		return float64(int32(binary.LittleEndian.Uint32(data[:4]))) / 2147483648, nil
	default:
		return 0, fmt.Errorf("unsupported PCM bit depth: %d", bytesPerSample*8)
	}
}
