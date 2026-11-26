// Package media decodes a video into downsampled field images for cadence
// analysis. ffmpeg is used only as a decoder; classification remains in Go.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/lsafjasf/20261127/cadence"
)

const (
	defaultFieldWidth  = 32
	defaultFieldHeight = 18
)

// Options configures decoding. Zero values use compact analysis defaults.
type Options struct {
	FFmpeg      string
	FFprobe     string
	FieldWidth  int
	FieldHeight int
}

type probeOutput struct {
	Streams []probeStream `json:"streams"`
}

type probeStream struct {
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	RFrameRate   string `json:"r_frame_rate"`
	AvgFrameRate string `json:"avg_frame_rate"`
	FieldOrder   string `json:"field_order"`
}

// Extract probes the first video stream and streams decoded fields from
// ffmpeg. A coded frame becomes Top followed by Bottom, regardless of the
// temporal field-order flag.
func Extract(ctx context.Context, path string, opts Options) (cadence.Media, error) {
	if strings.TrimSpace(path) == "" {
		return cadence.Media{}, fmt.Errorf("media: empty input path")
	}
	opts = normalizeOptions(opts)
	probe, err := probeVideo(ctx, opts.FFprobe, path)
	if err != nil {
		return cadence.Media{}, err
	}
	stream := probe.Streams[0]
	rate := parseRate(stream.AvgFrameRate)
	if rate <= 0 {
		rate = parseRate(stream.RFrameRate)
	}
	if rate <= 0 {
		return cadence.Media{}, fmt.Errorf("media: cannot determine video frame rate")
	}

	args := []string{
		"-v", "error",
		"-nostdin",
		"-i", path,
		"-map", "0:v:0",
		"-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("separatefields,scale=%d:%d:flags=area,format=gray,setsar=1",
			opts.FieldWidth, opts.FieldHeight),
		"-fps_mode", "passthrough",
		"-f", "rawvideo",
		"pipe:1",
	}
	cmd := exec.CommandContext(ctx, opts.FFmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return cadence.Media{}, fmt.Errorf("media: ffmpeg pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return cadence.Media{}, fmt.Errorf("media: start ffmpeg: %w", err)
	}

	media := cadence.Media{
		FrameRate:  rate,
		FieldOrder: hintFromMetadata(stream.FieldOrder),
	}
	fieldSize := opts.FieldWidth * opts.FieldHeight
	buf := make([]byte, fieldSize)
	var fields [2]cadence.Image
	fieldNo := 0
	for {
		if _, err := io.ReadFull(stdout, buf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return cadence.Media{}, fmt.Errorf("media: read ffmpeg output: %w", err)
		}
		pix := append([]uint8(nil), buf...)
		fields[fieldNo%2] = cadence.Image{
			Width:  opts.FieldWidth,
			Height: opts.FieldHeight,
			Pix:    pix,
		}
		if fieldNo%2 == 1 {
			media.Frames = append(media.Frames, cadence.Frame{
				Top:    fields[0],
				Bottom: fields[1],
			})
		}
		fieldNo++
	}
	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return cadence.Media{}, fmt.Errorf("media: ffmpeg decode failed: %s", msg)
	}
	if len(media.Frames) == 0 {
		return cadence.Media{}, fmt.Errorf("media: %w", cadence.ErrNoVideo)
	}
	if fieldNo%2 != 0 {
		return cadence.Media{}, fmt.Errorf("media: ffmpeg returned an odd number of fields (%d)", fieldNo)
	}
	return media, nil
}

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.FFmpeg) == "" {
		opts.FFmpeg = "ffmpeg"
	}
	if strings.TrimSpace(opts.FFprobe) == "" {
		opts.FFprobe = "ffprobe"
	}
	if opts.FieldWidth <= 0 {
		opts.FieldWidth = defaultFieldWidth
	}
	if opts.FieldHeight <= 0 {
		opts.FieldHeight = defaultFieldHeight
	}
	return opts
}

func probeVideo(ctx context.Context, ffprobe, path string) (probeOutput, error) {
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,avg_frame_rate,field_order",
		"-of", "json",
		"--", path,
	}
	cmd := exec.CommandContext(ctx, ffprobe, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return probeOutput{}, fmt.Errorf("media: ffprobe failed: %s", msg)
	}
	var probe probeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return probeOutput{}, fmt.Errorf("media: parse ffprobe output: %w", err)
	}
	if len(probe.Streams) == 0 {
		return probeOutput{}, fmt.Errorf("media: input has no video stream")
	}
	return probe, nil
}

func parseRate(value string) float64 {
	if value == "" || value == "0/0" {
		return 0
	}
	parts := strings.SplitN(value, "/", 2)
	num, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || num <= 0 {
		return 0
	}
	if len(parts) == 1 {
		return num
	}
	den, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || den <= 0 {
		return 0
	}
	rate := num / den
	if math.IsInf(rate, 0) || math.IsNaN(rate) {
		return 0
	}
	return rate
}

func hintFromMetadata(value string) cadence.FieldOrderHint {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tt", "tff", "top":
		return cadence.HintTFF
	case "bb", "bff", "bottom":
		return cadence.HintBFF
	case "progressive", "prog":
		return cadence.HintProgressive
	default:
		return cadence.HintUnknown
	}
}
