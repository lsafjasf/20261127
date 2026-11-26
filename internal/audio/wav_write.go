package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// WriteWAV writes a canonical PCM or IEEE-float WAV. More than two channels
// are emitted as WAVE_FORMAT_EXTENSIBLE with the buffer's channel mask.
func WriteWAV(path string, b *Buffer) error {
	raw, err := EncodeWAV(b)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// EncodeWAV serializes a Buffer without modifying it.
func EncodeWAV(b *Buffer) ([]byte, error) {
	f := b.Format
	if f.SampleRate <= 0 || f.Channels <= 0 || len(b.Samples) != f.Channels {
		return nil, fmt.Errorf("invalid buffer format")
	}
	bytesPerSample := (f.Bits + 7) / 8
	frames := len(b.Samples[0])
	for _, ch := range b.Samples {
		if len(ch) != frames {
			return nil, fmt.Errorf("channel lengths differ")
		}
	}
	if f.ChannelMask == 0 {
		f.ChannelMask = DefaultChannelMask(f.Channels)
	}
	dataLen := frames * f.Channels * bytesPerSample
	data := make([]byte, dataLen)
	for frame := 0; frame < frames; frame++ {
		for ch := 0; ch < f.Channels; ch++ {
			off := (frame*f.Channels + ch) * bytesPerSample
			encodeSample(data[off:off+bytesPerSample], b.Samples[ch][frame], f)
		}
	}

	extensible := f.Channels > 2
	fmtSize := 16
	if extensible {
		fmtSize = 40
	}
	pad := len(data) % 2
	out := make([]byte, 12+8+fmtSize+8+len(data)+pad)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], uint32(fmtSize))
	pos := 20
	if extensible {
		binary.LittleEndian.PutUint16(out[pos:pos+2], 0xfffe)
	} else {
		binary.LittleEndian.PutUint16(out[pos:pos+2], uint16(f.Encoding))
	}
	binary.LittleEndian.PutUint16(out[pos+2:pos+4], uint16(f.Channels))
	binary.LittleEndian.PutUint32(out[pos+4:pos+8], uint32(f.SampleRate))
	blockAlign := f.Channels * bytesPerSample
	binary.LittleEndian.PutUint32(out[pos+8:pos+12], uint32(f.SampleRate*blockAlign))
	binary.LittleEndian.PutUint16(out[pos+12:pos+14], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[pos+14:pos+16], uint16(f.Bits))
	if extensible {
		binary.LittleEndian.PutUint16(out[pos+16:pos+18], 22)
		binary.LittleEndian.PutUint16(out[pos+18:pos+20], uint16(f.Bits))
		binary.LittleEndian.PutUint32(out[pos+20:pos+24], f.ChannelMask)
		putSubformatGUID(out[pos+24:pos+40], f.Encoding)
	}
	pos += fmtSize
	copy(out[pos:pos+4], "data")
	binary.LittleEndian.PutUint32(out[pos+4:pos+8], uint32(dataLen))
	copy(out[pos+8:], data)
	return out, nil
}

func putSubformatGUID(dst []byte, enc Encoding) {
	binary.LittleEndian.PutUint32(dst[0:4], uint32(enc))
	copy(dst[4:], []byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71})
}

func encodeSample(dst []byte, sample float64, f Format) {
	if math.IsNaN(sample) || math.IsInf(sample, 0) {
		sample = 0
	}
	if f.Encoding == Float {
		if f.Bits == 32 {
			binary.LittleEndian.PutUint32(dst, math.Float32bits(float32(sample)))
		} else {
			binary.LittleEndian.PutUint64(dst, math.Float64bits(sample))
		}
		return
	}
	sample = math.Max(-1, math.Min(1, sample))
	switch f.Bits {
	case 8:
		dst[0] = byte(math.Round(sample*128) + 128)
	case 16:
		v := int64(math.Round(sample * 32768))
		v = max(-32768, min(32767, v))
		binary.LittleEndian.PutUint16(dst, uint16(int16(v)))
	case 24:
		v := int64(math.Round(sample * 8388608))
		v = max(-8388608, min(8388607, v))
		dst[0], dst[1], dst[2] = byte(v), byte(v>>8), byte(v>>16)
	case 32:
		v := int64(math.Round(sample * 2147483648))
		v = max(-2147483648, min(2147483647, v))
		binary.LittleEndian.PutUint32(dst, uint32(int32(v)))
	}
}
