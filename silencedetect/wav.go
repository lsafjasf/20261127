package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// Audio holds decoded PCM data. Samples are scaled to signed 32-bit full
// scale regardless of source bit depth, so dBFS math is uniform. Exact
// digital zeros remain zero after scaling.
type Audio struct {
	Path       string
	SampleRate uint32
	Channels   int
	Bits       int
	IsFloat    bool
	Frames     int
	// Samples[ch][i] is the sample of channel ch at frame i.
	Samples [][]int32 `json:"-"`
}

// audioMeta keeps JSON reports compact: decoded sample arrays stay in memory
// for analysis but are never serialized.
type audioMeta struct {
	Path        string  `json:"path"`
	SampleRate  uint32  `json:"sampleRate"`
	Channels    int     `json:"channels"`
	Bits        int     `json:"bits"`
	IsFloat     bool    `json:"isFloat"`
	Frames      int     `json:"frames"`
	DurationSec float64 `json:"durationSec"`
}

func (a *Audio) MarshalJSON() ([]byte, error) {
	return json.Marshal(audioMeta{
		Path:        a.Path,
		SampleRate:  a.SampleRate,
		Channels:    a.Channels,
		Bits:        a.Bits,
		IsFloat:     a.IsFloat,
		Frames:      a.Frames,
		DurationSec: float64(a.Frames) / float64(a.SampleRate),
	})
}

type waveFmt struct {
	tag      uint16
	channels uint16
	rate     uint32
	bits     uint16
}

var (
	errNotWAV    = errors.New("not a RIFF/WAVE file")
	errNoFmt     = errors.New("WAV missing fmt chunk")
	errNoData    = errors.New("WAV has no data chunk")
	errBigEndian = errors.New("RIFX big-endian WAVE is not supported")
)

// ParseWAV decodes a little-endian RIFF/WAVE byte slice.
func ParseWAV(path string, b []byte) (*Audio, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		if len(b) >= 4 && string(b[0:4]) == "RIFX" {
			return nil, errBigEndian
		}
		return nil, errNotWAV
	}

	var fmtInfo waveFmt
	gotFmt := false
	var data []byte

	off := 12
	for off+8 <= len(b) {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if body+size > len(b) {
			size = len(b) - body
		}
		next := body + size + size&1 // chunks are word-aligned

		switch id {
		case "fmt ":
			f, err := parseFmtChunk(b[body : body+size])
			if err != nil {
				return nil, err
			}
			fmtInfo = f
			gotFmt = true
		case "data":
			data = append(data, b[body:body+size]...)
		}
		off = next
	}

	if !gotFmt {
		return nil, errNoFmt
	}
	if len(data) == 0 {
		return nil, errNoData
	}

	return decodePCM(path, data, fmtInfo)
}

func parseFmtChunk(c []byte) (waveFmt, error) {
	if len(c) < 16 {
		return waveFmt{}, errors.New("fmt chunk too short")
	}
	f := waveFmt{
		tag:      binary.LittleEndian.Uint16(c[0:2]),
		channels: binary.LittleEndian.Uint16(c[2:4]),
		rate:     binary.LittleEndian.Uint32(c[4:8]),
		bits:     binary.LittleEndian.Uint16(c[14:16]),
	}
	// WAVEFORMATEXTENSIBLE: real format tag lives in the sub-format GUID.
	if f.tag == 0xFFFE && len(c) >= 40 {
		f.tag = binary.LittleEndian.Uint16(c[24:26])
	}
	if f.channels == 0 || f.rate == 0 {
		return waveFmt{}, errors.New("invalid fmt: zero channels or sample rate")
	}
	return f, nil
}

func decodePCM(path string, data []byte, f waveFmt) (*Audio, error) {
	isFloat := f.tag == 3
	if f.tag != 1 && !isFloat {
		return nil, fmt.Errorf("unsupported WAV format tag %d (only PCM and IEEE float)", f.tag)
	}

	bytesPerSample := int(f.bits) / 8
	if bytesPerSample <= 0 || int(f.bits)%8 != 0 {
		return nil, fmt.Errorf("unsupported bit depth %d", f.bits)
	}
	frameBytes := bytesPerSample * int(f.channels)
	if frameBytes == 0 || len(data) < frameBytes {
		return nil, errors.New("data chunk shorter than one frame")
	}
	frames := len(data) / frameBytes

	out := &Audio{
		Path:       path,
		SampleRate: f.rate,
		Channels:   int(f.channels),
		Bits:       int(f.bits),
		IsFloat:    isFloat,
		Frames:     frames,
		Samples:    make([][]int32, f.channels),
	}
	for ch := range out.Samples {
		out.Samples[ch] = make([]int32, frames)
	}

	for i := 0; i < frames; i++ {
		base := i * frameBytes
		for ch := 0; ch < int(f.channels); ch++ {
			p := base + ch*bytesPerSample
			s, err := decodeSample(data[p:p+bytesPerSample], int(f.bits), isFloat)
			if err != nil {
				return nil, err
			}
			out.Samples[ch][i] = s
		}
	}
	return out, nil
}

func decodeSample(b []byte, bits int, isFloat bool) (int32, error) {
	switch {
	case isFloat && bits == 32:
		return floatToInt32(float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))), nil
	case isFloat && bits == 64:
		return floatToInt32(math.Float64frombits(binary.LittleEndian.Uint64(b))), nil
	case isFloat:
		return 0, fmt.Errorf("unsupported float bit depth %d", bits)
	case bits == 8:
		// WAV 8-bit PCM is unsigned, centered at 128.
		return int32(b[0]-128) << 24, nil
	case bits == 16:
		return int32(int16(binary.LittleEndian.Uint16(b))) << 16, nil
	case bits == 24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		return v << 8, nil
	case bits == 32:
		return int32(binary.LittleEndian.Uint32(b)), nil
	default:
		return 0, fmt.Errorf("unsupported bit depth %d", bits)
	}
}

func floatToInt32(v float64) int32 {
	const scale = float64(1<<31 - 1)
	v *= scale
	if v >= scale {
		return math.MaxInt32
	}
	if v <= -scale {
		return math.MinInt32
	}
	return int32(v)
}
