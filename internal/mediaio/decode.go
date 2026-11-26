package mediaio

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os/exec"
	"strconv"

	"proalpha/internal/imagex"
)

var ErrFFmpegNotFound = errors.New("ffmpeg not found in PATH")

type DecodeOptions struct {
	MaxFrames int
	Width     int
	Height    int
}

func FFmpegPath() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", ErrFFmpegNotFound
	}
	return path, nil
}

func DecodeRGBA64(path string, options DecodeOptions) ([]*image.NRGBA64, error) {
	bin, err := FFmpegPath()
	if err != nil {
		return nil, err
	}

	args := []string{"-v", "error", "-i", path, "-map", "0:v:0"}
	filter := "format=rgba64le"
	if options.Width > 0 && options.Height > 0 {
		filter = fmt.Sprintf("scale=%d:%d:flags=bilinear,setsar=1,%s",
			options.Width, options.Height, filter)
	}
	args = append(args, "-vf", filter)
	if options.MaxFrames > 0 {
		args = append(args, "-frames:v", strconv.Itoa(options.MaxFrames))
	}
	args = append(args, "-f", "rawvideo", "-pix_fmt", "rgba64le", "pipe:1")

	cmd := exec.Command(bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	width, height := options.Width, options.Height
	if width <= 0 || height <= 0 {
		return nil, errors.New("decode dimensions must be supplied")
	}
	frames, readErr := imagex.ReadRawRGBA64(stdout, width, height, options.MaxFrames)
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, fmt.Errorf("read ffmpeg raw video: %w (%s)", readErr, stderr.String())
	}
	if waitErr != nil {
		return nil, fmt.Errorf("ffmpeg decode failed: %w (%s)", waitErr, stderr.String())
	}
	return frames, nil
}
