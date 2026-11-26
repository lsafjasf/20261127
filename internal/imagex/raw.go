package imagex

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// ReadRawRGBA64 reads concatenated FFmpeg rgba64le frames as straight NRGBA64.
func ReadRawRGBA64(r io.Reader, width, height, maxFrames int) ([]*image.NRGBA64, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid frame dimensions %dx%d", width, height)
	}
	frameSize := width * height * 8
	frames := make([]*image.NRGBA64, 0)

	for maxFrames <= 0 || len(frames) < maxFrames {
		raw := make([]byte, frameSize)
		n, err := io.ReadFull(r, raw)
		if err == io.EOF && n == 0 {
			break
		}
		if err == io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("truncated raw frame: got %d/%d bytes", n, frameSize)
		}
		if err != nil {
			return nil, err
		}

		frame := image.NewNRGBA64(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				src := (y*width + x) * 8
				dst := frame.PixOffset(x, y)
				value := binary.LittleEndian.Uint16(raw[src : src+2])
				binary.BigEndian.PutUint16(frame.Pix[dst+0:dst+2], value)
				value = binary.LittleEndian.Uint16(raw[src+2 : src+4])
				binary.BigEndian.PutUint16(frame.Pix[dst+2:dst+4], value)
				value = binary.LittleEndian.Uint16(raw[src+4 : src+6])
				binary.BigEndian.PutUint16(frame.Pix[dst+4:dst+6], value)
				value = binary.LittleEndian.Uint16(raw[src+6 : src+8])
				binary.BigEndian.PutUint16(frame.Pix[dst+6:dst+8], value)
			}
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

func SavePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := encoder.Encode(file, img); err != nil {
		return err
	}
	return file.Close()
}

func CloneRGBA64(src *image.NRGBA64) *image.NRGBA64 {
	if src == nil {
		return nil
	}
	dst := image.NewNRGBA64(src.Rect)
	copy(dst.Pix, src.Pix)
	return dst
}
