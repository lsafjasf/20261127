package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodeSample(t *testing.T) {
	cases := []struct {
		name   string
		bits   int
		float  bool
		bytes  []byte
		want32 int32
	}{
		{"8-bit mid", 8, false, []byte{128}, 0},
		{"8-bit max", 8, false, []byte{255}, 127 << 24},
		{"16-bit min", 16, false, []byte{0x00, 0x80}, math.MinInt32},
		{"24-bit one", 24, false, []byte{0x01, 0x00, 0x00}, 1 << 8},
		{"32-bit max", 32, false, []byte{0xff, 0xff, 0xff, 0x7f}, math.MaxInt32},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decodeSample(c.bytes, c.bits, c.float)
			if err != nil {
				t.Fatalf("decodeSample: %v", err)
			}
			if got != c.want32 {
				t.Fatalf("got %d, want %d", got, c.want32)
			}
		})
	}

	// float32 zero stays exactly zero (digital silence must survive).
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], math.Float32bits(0))
	got, err := decodeSample(buf[:], 32, true)
	if err != nil || got != 0 {
		t.Fatalf("float32 zero: got %d err %v", got, err)
	}
}

func TestParseWAVRoundTrip(t *testing.T) {
	rate := 8000
	frames := rate / 10 // 100 ms
	src := makeStereoSilence(frames)
	// put a known sample in channel 1
	src[1][10] = int32(1000) << 16
	b := buildWAV(t, uint32(rate), 16, src)

	a, err := ParseWAV("mem.wav", b)
	if err != nil {
		t.Fatalf("ParseWAV: %v", err)
	}
	if a.SampleRate != uint32(rate) || a.Channels != 2 || a.Bits != 16 || a.Frames != frames {
		t.Fatalf("header mismatch: %+v", a)
	}
	if a.Samples[1][10] != int32(1000)<<16 {
		t.Fatalf("sample mismatch: %d", a.Samples[1][10])
	}
}

func TestParseWAVIgnoresJunkChunks(t *testing.T) {
	rate := 8000
	src := makeStereoSilence(rate / 20)
	b := buildWAV(t, uint32(rate), 16, src)

	// Insert an odd-sized junk chunk between RIFF header and fmt.
	var out bytes.Buffer
	out.Write(b[:12])
	junk := []byte("junk")
	junk = append(junk, binary.LittleEndian.AppendUint32(nil, 3)...)
	junk = append(junk, 1, 2, 3, 0) // 3 bytes + pad
	out.Write(junk)
	out.Write(b[12:])

	a, err := ParseWAV("junk.wav", out.Bytes())
	if err != nil {
		t.Fatalf("ParseWAV with junk chunk: %v", err)
	}
	if a.Frames != rate/20 {
		t.Fatalf("frames = %d", a.Frames)
	}
}

// buildWAV serializes interleaved 16-bit PCM stereo with a standard layout.
func buildWAV(t *testing.T, rate uint32, bits int, ch [][]int32) []byte {
	t.Helper()
	channels := len(ch)
	frames := len(ch[0])
	bytesPerSample := bits / 8
	dataSize := frames * channels * bytesPerSample

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(channels))
	binary.Write(&buf, binary.LittleEndian, rate)
	binary.Write(&buf, binary.LittleEndian, rate*uint32(channels)*uint32(bytesPerSample))
	binary.Write(&buf, binary.LittleEndian, uint16(channels*bytesPerSample))
	binary.Write(&buf, binary.LittleEndian, uint16(bits))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataSize))

	for i := 0; i < frames; i++ {
		for c := 0; c < channels; c++ {
			s := ch[c][i] >> 16 // back to 16-bit
			binary.Write(&buf, binary.LittleEndian, int16(s))
		}
	}
	return buf.Bytes()
}

func makeStereoSilence(frames int) [][]int32 {
	ch := make([][]int32, 2)
	ch[0] = make([]int32, frames)
	ch[1] = make([]int32, frames)
	return ch
}
