package wav

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodePCM16Stereo(t *testing.T) {
	left := []int16{0, 16384, -16384, 32767}
	right := []int16{-32768, 0, 8192, -8192}
	var pcm bytes.Buffer
	for i := range left {
		_ = binary.Write(&pcm, binary.LittleEndian, left[i])
		_ = binary.Write(&pcm, binary.LittleEndian, right[i])
	}
	a, err := Decode(bytes.NewReader(makeWAV(1, 2, 44100, 16, pcm.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if a.Channels != 2 || a.SampleRate != 44100 || a.Frames != len(left) {
		t.Fatalf("unexpected format: %+v", a)
	}
	if math.Abs(a.Samples[0]) > 1e-12 || math.Abs(a.Samples[1]-(-1)) > 1e-12 {
		t.Fatalf("unexpected decoded samples: %v", a.Samples[:2])
	}
	if math.Abs(a.Samples[5]-0.25) > 1e-6 {
		t.Fatalf("right[2] = %v, want 0.25", a.Samples[5])
	}
}

func TestDecodePCM24(t *testing.T) {
	pcm := []byte{0x00, 0x00, 0x40, 0x00, 0x00, 0xc0}
	a, err := Decode(bytes.NewReader(makeWAV(1, 1, 48000, 24, pcm)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Frames != 2 {
		t.Fatalf("frames = %d, want 2", a.Frames)
	}
	if math.Abs(a.Samples[0]-0.5) > 1e-6 || math.Abs(a.Samples[1]-(-0.5)) > 1e-6 {
		t.Fatalf("decoded = %v, want [0.5 -0.5]", a.Samples)
	}
}

func TestDecodeFloat32AndExtensible(t *testing.T) {
	var pcm bytes.Buffer
	for _, v := range []float32{0.25, -0.75} {
		_ = binary.Write(&pcm, binary.LittleEndian, math.Float32bits(v))
	}
	data := makeWAV(formatExtensible, 1, 96000, 32, pcm.Bytes())
	// fmt chunk starts at 20. Set cbSize and the first two bytes of the
	// SubFormat GUID to IEEE float.
	binary.LittleEndian.PutUint16(data[36:38], 22)
	binary.LittleEndian.PutUint16(data[44:46], formatFloat)
	a, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if a.Frames != 2 || a.SampleRate != 96000 {
		t.Fatalf("unexpected format: %+v", a)
	}
	if math.Abs(a.Samples[0]-0.25) > 1e-7 || math.Abs(a.Samples[1]-(-0.75)) > 1e-7 {
		t.Fatalf("decoded = %v", a.Samples)
	}
}

func TestRejectsTruncatedDataChunk(t *testing.T) {
	data := makeWAV(1, 1, 8000, 16, []byte{1, 2, 3})
	if _, err := Decode(bytes.NewReader(data)); err == nil {
		t.Fatal("expected an error for a partial frame")
	}
}

func makeWAV(format uint16, channels int, rate, bits int, pcm []byte) []byte {
	blockAlign := channels * bits / 8
	fmtSize := 16
	if format == formatExtensible {
		fmtSize = 40
	}
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(4+8+fmtSize+8+len(pcm)))
	out.WriteString("WAVE")
	out.WriteString("fmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(fmtSize))
	_ = binary.Write(&out, binary.LittleEndian, format)
	_ = binary.Write(&out, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&out, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(rate*blockAlign))
	_ = binary.Write(&out, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&out, binary.LittleEndian, uint16(bits))
	if format == formatExtensible {
		_ = binary.Write(&out, binary.LittleEndian, uint16(22))
		_ = binary.Write(&out, binary.LittleEndian, uint16(bits))
		_ = binary.Write(&out, binary.LittleEndian, uint32(0))
		out.Write(make([]byte, 16))
	}
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(pcm)))
	out.Write(pcm)
	return out.Bytes()
}
