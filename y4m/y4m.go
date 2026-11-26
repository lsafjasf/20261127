// Package y4m reads uncompressed YUV4MPEG2 streams using only the standard
// library. Only the luminance plane is retained by the returned frames.
package y4m

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Frame struct {
	Luma      []byte
	Width     int
	Height    int
	Timestamp float64
}

type Reader struct {
	br         *bufio.Reader
	width      int
	height     int
	fps        float64
	chromaSize int
	frameIndex int
}

func NewReader(r io.Reader) (*Reader, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read Y4M header: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 || fields[0] != "YUV4MPEG2" {
		return nil, fmt.Errorf("input is not a Y4M stream")
	}
	width, height, fps := 0, 0, 25.0
	chroma := "420jpeg"
	for _, field := range fields[1:] {
		switch {
		case strings.HasPrefix(field, "W"):
			width, err = strconv.Atoi(field[1:])
		case strings.HasPrefix(field, "H"):
			height, err = strconv.Atoi(field[1:])
		case strings.HasPrefix(field, "F"):
			fps, err = parseFrameRate(field[1:])
		case strings.HasPrefix(field, "C"):
			chroma = field[1:]
		}
		if err != nil {
			return nil, fmt.Errorf("parse Y4M header field %q: %w", field, err)
		}
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid Y4M dimensions %dx%d", width, height)
	}
	if fps <= 0 {
		return nil, fmt.Errorf("invalid Y4M frame rate %v", fps)
	}
	chromaSize, err := chromaBytes(chroma, width, height)
	if err != nil {
		return nil, err
	}
	return &Reader{
		br:         br,
		width:      width,
		height:     height,
		fps:        fps,
		chromaSize: chromaSize,
	}, nil
}

func (r *Reader) Width() int      { return r.width }
func (r *Reader) Height() int     { return r.height }
func (r *Reader) FPS() float64    { return r.fps }
func (r *Reader) FrameIndex() int { return r.frameIndex }

func (r *Reader) Next() (Frame, error) {
	line, err := r.br.ReadString('\n')
	if err == io.EOF && line == "" {
		return Frame{}, io.EOF
	}
	if err != nil {
		return Frame{}, fmt.Errorf("read frame header: %w", err)
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "FRAME") {
		return Frame{}, fmt.Errorf("invalid Y4M frame header %q", line)
	}
	luma := make([]byte, r.width*r.height)
	if _, err := io.ReadFull(r.br, luma); err != nil {
		return Frame{}, fmt.Errorf("read luma frame %d: %w", r.frameIndex, err)
	}
	if r.chromaSize > 0 {
		if _, err := io.CopyN(io.Discard, r.br, int64(r.chromaSize)); err != nil {
			return Frame{}, fmt.Errorf("skip chroma frame %d: %w", r.frameIndex, err)
		}
	}
	frame := Frame{
		Luma:      luma,
		Width:     r.width,
		Height:    r.height,
		Timestamp: float64(r.frameIndex) / r.fps,
	}
	r.frameIndex++
	return frame, nil
}

func parseFrameRate(s string) (float64, error) {
	parts := strings.SplitN(s, ":", 2)
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	if len(parts) == 1 {
		return numerator, nil
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, err
	}
	if denominator == 0 {
		return 0, fmt.Errorf("zero frame-rate denominator")
	}
	return numerator / denominator, nil
}

func chromaBytes(name string, width, height int) (int, error) {
	switch name {
	case "420", "420jpeg", "420mpeg2", "420paldv":
		return ((width + 1) / 2) * ((height + 1) / 2) * 2, nil
	case "422":
		return ((width + 1) / 2) * height * 2, nil
	case "444":
		return width * height * 2, nil
	case "mono":
		return 0, nil
	default:
		return 0, fmt.Errorf("unsupported Y4M chroma format %q", name)
	}
}
