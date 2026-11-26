// Package audio contains the multichannel WAV and low-frequency analysis
// primitives used by lfecheck.
package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// Encoding identifies the sample representation stored in a WAV file.
type Encoding uint16

const (
	PCM   Encoding = 1
	Float Encoding = 3
)

// Format describes the sample layout of a Buffer.
type Format struct {
	SampleRate  int
	Channels    int
	Bits        int
	Encoding    Encoding
	ChannelMask uint32
}

// Buffer stores planar, normalized audio samples. Every channel has the same
// length as Samples.
type Buffer struct {
	Format  Format
	Samples [][]float64
}

// Duration returns the duration of the buffer in seconds.
func (b *Buffer) Duration() float64 {
	if b.Format.SampleRate == 0 || len(b.Samples) == 0 {
		return 0
	}
	return float64(len(b.Samples[0])) / float64(b.Format.SampleRate)
}

// ReadWAV decodes an integer or IEEE-float RIFF/WAVE file. Sample values are
// normalized to approximately [-1, 1].
func ReadWAV(path string) (*Buffer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b, err := DecodeWAV(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

// DecodeWAV parses a canonical RIFF/WAVE byte stream.
func DecodeWAV(raw []byte) (*Buffer, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a RIFF/WAVE file")
	}
	var format Format
	var data []byte
	for pos := 12; pos+8 <= len(raw); {
		id := string(raw[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(raw[pos+4 : pos+8]))
		start := pos + 8
		end := start + size
		if size < 0 || end > len(raw) || end < start {
			return nil, fmt.Errorf("invalid %q chunk size %d", id, size)
		}
		switch id {
		case "fmt ":
			f, err := decodeFormat(raw[start:end])
			if err != nil {
				return nil, err
			}
			format = f
		case "data":
			data = raw[start:end]
		}
		pos = end + size%2
	}
	if format.SampleRate <= 0 || format.Channels <= 0 || format.Bits <= 0 {
		return nil, fmt.Errorf("missing or invalid fmt chunk")
	}
	if data == nil {
		return nil, fmt.Errorf("missing data chunk")
	}
	bytesPerSample := (format.Bits + 7) / 8
	frameBytes := bytesPerSample * format.Channels
	if frameBytes == 0 || len(data)%frameBytes != 0 {
		return nil, fmt.Errorf("data size %d is not frame aligned (%d bytes/frame)", len(data), frameBytes)
	}
	frames := len(data) / frameBytes
	channels := make([][]float64, format.Channels)
	for ch := range channels {
		channels[ch] = make([]float64, frames)
	}
	for frame := 0; frame < frames; frame++ {
		base := frame * frameBytes
		for ch := 0; ch < format.Channels; ch++ {
			off := base + ch*bytesPerSample
			v, err := decodeSample(data[off:off+bytesPerSample], format)
			if err != nil {
				return nil, err
			}
			channels[ch][frame] = v
		}
	}
	return &Buffer{Format: format, Samples: channels}, nil
}

func decodeFormat(raw []byte) (Format, error) {
	if len(raw) < 16 {
		return Format{}, fmt.Errorf("short fmt chunk")
	}
	tag := binary.LittleEndian.Uint16(raw[0:2])
	f := Format{
		Channels:   int(binary.LittleEndian.Uint16(raw[2:4])),
		SampleRate: int(binary.LittleEndian.Uint32(raw[4:8])),
		Bits:       int(binary.LittleEndian.Uint16(raw[14:16])),
	}
	switch tag {
	case uint16(PCM), uint16(Float):
		f.Encoding = Encoding(tag)
	case 0xfffe:
		if len(raw) < 40 {
			return Format{}, fmt.Errorf("short WAVE_FORMAT_EXTENSIBLE fmt chunk")
		}
		f.Encoding = Encoding(binary.LittleEndian.Uint16(raw[24:26]))
		f.ChannelMask = binary.LittleEndian.Uint32(raw[20:24])
	default:
		return Format{}, fmt.Errorf("unsupported WAV format tag 0x%04x", tag)
	}
	if f.Encoding != PCM && f.Encoding != Float {
		return Format{}, fmt.Errorf("unsupported sub-format 0x%04x", f.Encoding)
	}
	if f.Encoding == Float && f.Bits != 32 && f.Bits != 64 {
		return Format{}, fmt.Errorf("unsupported float width %d", f.Bits)
	}
	if f.Encoding == PCM && f.Bits != 8 && f.Bits != 16 && f.Bits != 24 && f.Bits != 32 {
		return Format{}, fmt.Errorf("unsupported PCM width %d", f.Bits)
	}
	return f, nil
}

func decodeSample(raw []byte, f Format) (float64, error) {
	if f.Encoding == Float {
		if f.Bits == 32 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(raw))), nil
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(raw)), nil
	}
	switch f.Bits {
	case 8:
		return (float64(raw[0]) - 128) / 128, nil
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(raw))) / 32768, nil
	case 24:
		v := int32(raw[0]) | int32(raw[1])<<8 | int32(raw[2])<<16
		if v&0x800000 != 0 {
			v |= ^0xffffff
		}
		return float64(v) / 8388608, nil
	case 32:
		return float64(int32(binary.LittleEndian.Uint32(raw))) / 2147483648, nil
	}
	return 0, fmt.Errorf("unsupported sample width %d", f.Bits)
}
