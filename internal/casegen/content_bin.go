package casegen

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// mulberry32 is a tiny deterministic PRNG so fixtures are reproducible.
type mulberry32 struct{ state uint32 }

func (m *mulberry32) next() byte {
	m.state += 0x6D2B79F5
	z := m.state
	z = (z ^ (z >> 15)) * (z | 1)
	z ^= z + (z^(z>>7))*(z|61)
	return byte((z ^ (z >> 14)) & 0xff)
}

func ev003FirmwareBlob() []byte {
	var buf bytes.Buffer
	buf.WriteString("ECFW1-FIXTURE-v1\n")
	header := make([]byte, 48)
	for i := range header {
		header[i] = 0xff
	}
	copy(header[0:7], []byte("ECFW1\x00\x01"))
	buf.Write(header)
	rng := &mulberry32{state: 0x5eed1234}
	body := make([]byte, 4096)
	for i := range body {
		body[i] = rng.next()
	}
	buf.Write(body)
	return buf.Bytes()
}

func unknownCacheBlob() []byte {
	rng := &mulberry32{state: 0x0badf00d}
	b := make([]byte, 896)
	for i := range b {
		b[i] = rng.next()
	}
	return b
}

// ev004CapturePNG renders a small deterministic synthetic "camera capture".
func ev004CapturePNG(compression png.CompressionLevel) ([]byte, error) {
	const n = 96
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			r := uint8(20 + x*2)
			g := uint8(40 + y*2)
			b := uint8(90 + (x+y)/2)
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	for i := 0; i < n; i++ {
		img.SetRGBA(i, i, color.RGBA{R: 240, G: 240, B: 240, A: 255})
		if i+1 < n {
			img.SetRGBA(i+1, i, color.RGBA{R: 220, G: 30, B: 30, A: 255})
		}
	}
	for y := 30; y < 66; y++ {
		for x := 28; x < 68; x++ {
			dx, dy := x-48, y-48
			if dx*dx+dy*dy <= 18*18 {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 210, B: 60, A: 255})
			}
		}
	}
	for x := 8; x < 88; x++ {
		img.SetRGBA(x, 78, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: compression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// reencodePNG decodes src and re-encodes at a different compression level,
// yielding different bytes but identical pixels.
func reencodePNG(src []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
