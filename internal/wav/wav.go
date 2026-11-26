// Package wav decodes RIFF/WAVE files into normalized floating-point samples.
package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	formatPCM        = 0x0001
	formatFloat      = 0x0003
	formatExtensible = 0xfffe
)

// Audio is an interleaved, normalized sample buffer.
type Audio struct {
	SampleRate int
	Channels   int
	Frames     int
	Samples    []float64
}

// ReadFile decodes a RIFF/WAVE file. PCM and IEEE-float formats with 8, 16,
// 24, 32, or 64 bits per sample are supported.
func ReadFile(path string) (*Audio, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	a, err := Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

// Decode reads a RIFF/WAVE stream from r.
func Decode(r io.Reader) (*Audio, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}

	var format *fmtChunk
	var pcm []byte
	for off := 12; off+8 <= len(data); {
		id := string(data[off : off+4])
		size := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		start := off + 8
		end := start + size
		if size < 0 || end < start || end > len(data) {
			return nil, fmt.Errorf("invalid %q chunk size %d", id, size)
		}
		switch id {
		case "fmt ":
			format, err = parseFmt(data[start:end])
			if err != nil {
				return nil, err
			}
		case "data":
			pcm = data[start:end]
		}
		off = end + (size & 1)
	}
	if format == nil {
		return nil, errors.New("missing fmt chunk")
	}
	if pcm == nil {
		return nil, errors.New("missing data chunk")
	}
	return decodeSamples(format, pcm)
}

type fmtChunk struct {
	format        uint16
	channels      int
	sampleRate    int
	blockAlign    int
	bitsPerSample int
}

func parseFmt(b []byte) (*fmtChunk, error) {
	if len(b) < 16 {
		return nil, errors.New("short fmt chunk")
	}
	f := &fmtChunk{
		format:        binary.LittleEndian.Uint16(b[0:2]),
		channels:      int(binary.LittleEndian.Uint16(b[2:4])),
		sampleRate:    int(binary.LittleEndian.Uint32(b[4:8])),
		blockAlign:    int(binary.LittleEndian.Uint16(b[12:14])),
		bitsPerSample: int(binary.LittleEndian.Uint16(b[14:16])),
	}
	if f.channels <= 0 || f.sampleRate <= 0 {
		return nil, fmt.Errorf("invalid channels/sample rate: %d/%d", f.channels, f.sampleRate)
	}
	if f.format == formatExtensible {
		if len(b) < 40 {
			return nil, errors.New("short WAVE_FORMAT_EXTENSIBLE fmt chunk")
		}
		f.format = binary.LittleEndian.Uint16(b[24:26])
	}
	if f.format != formatPCM && f.format != formatFloat {
		return nil, fmt.Errorf("unsupported WAVE format 0x%04x", f.format)
	}
	bytesPerSample := (f.bitsPerSample + 7) / 8
	if bytesPerSample == 0 || f.blockAlign != f.channels*bytesPerSample {
		return nil, fmt.Errorf("invalid block alignment %d for %d channel(s), %d-bit samples",
			f.blockAlign, f.channels, f.bitsPerSample)
	}
	if f.format == formatFloat && f.bitsPerSample != 32 && f.bitsPerSample != 64 {
		return nil, fmt.Errorf("unsupported float sample width %d", f.bitsPerSample)
	}
	if f.format == formatPCM && f.bitsPerSample != 8 && f.bitsPerSample != 16 &&
		f.bitsPerSample != 24 && f.bitsPerSample != 32 {
		return nil, fmt.Errorf("unsupported PCM sample width %d", f.bitsPerSample)
	}
	return f, nil
}

func decodeSamples(f *fmtChunk, pcm []byte) (*Audio, error) {
	if len(pcm)%f.blockAlign != 0 {
		return nil, fmt.Errorf("data size %d is not a whole number of frames", len(pcm))
	}
	frames := len(pcm) / f.blockAlign
	samples := make([]float64, frames*f.channels)
	width := f.bitsPerSample / 8

	for frame := 0; frame < frames; frame++ {
		frameData := pcm[frame*f.blockAlign : (frame+1)*f.blockAlign]
		for ch := 0; ch < f.channels; ch++ {
			s, err := decodeSample(f, frameData[ch*width:(ch+1)*width])
			if err != nil {
				return nil, err
			}
			samples[frame*f.channels+ch] = s
		}
	}
	return &Audio{SampleRate: f.sampleRate, Channels: f.channels, Frames: frames, Samples: samples}, nil
}

func decodeSample(f *fmtChunk, b []byte) (float64, error) {
	if f.format == formatFloat {
		if f.bitsPerSample == 32 {
			v := float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
			return sanitizeFloat(v), nil
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(b))
		return sanitizeFloat(v), nil
	}

	switch f.bitsPerSample {
	case 8:
		return (float64(b[0]) - 128) / 128, nil
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768, nil
	case 24:
		u := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
		if u&0x800000 != 0 {
			u |= 0xff000000
		}
		return float64(int32(u)) / 8388608, nil
	case 32:
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648, nil
	default:
		return 0, fmt.Errorf("unsupported PCM sample width %d", f.bitsPerSample)
	}
}

func sanitizeFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Max(-1, math.Min(1, v))
}
