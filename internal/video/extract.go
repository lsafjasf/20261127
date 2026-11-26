package video

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type ExtractOptions struct {
	FFmpeg     string
	Width      int
	Height     int
	Range      Range
	FPS        float64
	Seconds    float64
	MaxFrames  int
	Start      float64
	SingleTime *float64
}

func ExtractFrames(input string, opts ExtractOptions) ([]*Frame, error) {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if opts.SingleTime != nil {
		args = append(args, "-ss", strconv.FormatFloat(*opts.SingleTime, 'f', 4, 64))
	} else if opts.Start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(opts.Start, 'f', 4, 64))
	}
	args = append(args, "-i", input, "-map", "0:v:0", "-an", "-sn", "-dn")
	if opts.SingleTime != nil {
		args = append(args, "-frames:v", "1")
	} else {
		filter := "fps=" + strconv.FormatFloat(opts.FPS, 'f', 6, 64) + ",format=yuv444p"
		args = append(args, "-vf", filter, "-frames:v", strconv.Itoa(opts.MaxFrames))
		if opts.Seconds > 0 {
			args = append(args, "-t", strconv.FormatFloat(opts.Seconds, 'f', 3, 64))
		}
	}
	args = append(args, "-pix_fmt", "yuv444p", "-f", "rawvideo", "pipe:1")

	cmd := exec.Command(opts.FFmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg extract: %w: %s", err, tail(stderr.String(), 800))
	}
	frameSize := opts.Width * opts.Height * 3
	if frameSize <= 0 || len(out) < frameSize {
		return nil, fmt.Errorf("ffmpeg returned %d bytes, need at least %d", len(out), frameSize)
	}
	count := len(out) / frameSize
	if opts.SingleTime != nil && count > 1 {
		count = 1
	}
	frames := make([]*Frame, 0, count)
	for i := 0; i < count; i++ {
		frame, err := NewFrame(opts.Width, opts.Height, out[i*frameSize:(i+1)*frameSize], opts.Range)
		if err != nil {
			return nil, err
		}
		if opts.SingleTime != nil {
			frame.Time = *opts.SingleTime
		} else {
			frame.Time = opts.Start + float64(i)/opts.FPS
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
