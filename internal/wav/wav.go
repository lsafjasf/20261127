package wav

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

type Audio struct {
	SampleRate int
	Channels   int
	Frames     int
	Samples    [][]float64
}

type format struct {
	tag        uint16
	channels   uint16
	sampleRate uint32
	bits       uint16
}

func ReadFile(path string) (*Audio, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Read(b)
}

func Read(b []byte) (*Audio, error) {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a RIFF/WAVE file")
	}

	var fmtInfo format
	var data []byte
	pos := 12
	for pos+8 <= len(b) {
		id := string(b[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(b) {
			return nil, fmt.Errorf("truncated %q chunk", id)
		}
		if id == "fmt " {
			if size < 16 {
				return nil, fmt.Errorf("short fmt chunk")
			}
			fmtInfo = format{
				tag:        binary.LittleEndian.Uint16(b[body : body+2]),
				channels:   binary.LittleEndian.Uint16(b[body+2 : body+4]),
				sampleRate: binary.LittleEndian.Uint32(b[body+4 : body+8]),
				bits:       binary.LittleEndian.Uint16(b[body+14 : body+16]),
			}
			if fmtInfo.tag == 0xFFFE && size >= 40 {
				fmtInfo.tag = binary.LittleEndian.Uint16(b[body+24 : body+26])
			}
		} else if id == "data" {
			data = b[body : body+size]
		}
		pos = body + size + size%2
	}
	if fmtInfo.channels == 0 || fmtInfo.sampleRate == 0 || fmtInfo.bits == 0 {
		return nil, fmt.Errorf("missing fmt chunk")
	}
	if data == nil {
		return nil, fmt.Errorf("missing data chunk")
	}
	if fmtInfo.tag != 1 && fmtInfo.tag != 3 {
		return nil, fmt.Errorf("unsupported WAV format tag %d", fmtInfo.tag)
	}

	ch := int(fmtInfo.channels)
	bytesPerSample := int(fmtInfo.bits / 8)
	frameBytes := bytesPerSample * ch
	if frameBytes == 0 || len(data) < frameBytes {
		return nil, fmt.Errorf("invalid PCM data")
	}
	frames := len(data) / frameBytes
	a := &Audio{
		SampleRate: int(fmtInfo.sampleRate),
		Channels:   ch,
		Frames:     frames,
		Samples:    make([][]float64, ch),
	}
	for c := range a.Samples {
		a.Samples[c] = make([]float64, frames)
	}

	for i := 0; i < frames; i++ {
		base := i * frameBytes
		for c := 0; c < ch; c++ {
			p := base + c*bytesPerSample
			a.Samples[c][i] = decodeSample(data[p:p+bytesPerSample], fmtInfo.bits, fmtInfo.tag)
		}
	}
	return a, nil
}

func decodeSample(b []byte, bits uint16, tag uint16) float64 {
	switch bits {
	case 8:
		return (float64(b[0]) - 128) / 128
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	case 24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if v&0x800000 != 0 {
			v |= -1 << 24
		}
		return float64(v) / 8388608
	case 32:
		if tag == 3 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		}
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	case 64:
		return math.Float64frombits(binary.LittleEndian.Uint64(b))
	default:
		return 0
	}
}

func WritePCM16(path string, a *Audio) error {
	if a.Channels <= 0 || len(a.Samples) != a.Channels {
		return fmt.Errorf("invalid audio channels")
	}
	frameBytes := 2 * a.Channels
	dataSize := a.Frames * frameBytes
	out := make([]byte, 44+dataSize)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], uint16(a.Channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(a.SampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(a.SampleRate*frameBytes))
	binary.LittleEndian.PutUint16(out[32:34], uint16(frameBytes))
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataSize))

	for i := 0; i < a.Frames; i++ {
		for c := 0; c < a.Channels; c++ {
			v := a.Samples[c][i]
			v = math.Max(-1, math.Min(1, v))
			n := int16(math.Round(v * 32767))
			off := 44 + i*frameBytes + c*2
			binary.LittleEndian.PutUint16(out[off:off+2], uint16(n))
		}
	}
	return os.WriteFile(path, out, 0o644)
}
