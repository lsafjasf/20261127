package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"colorfix/internal/color"
)

type Stream struct {
	Index          int               `json:"index"`
	CodecName      string            `json:"codec_name"`
	CodecLongName  string            `json:"codec_long_name"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	PixFmt         string            `json:"pix_fmt"`
	ColorRange     string            `json:"color_range"`
	ColorSpace     string            `json:"color_space"`
	ColorTransfer  string            `json:"color_transfer"`
	ColorPrimaries string            `json:"color_primaries"`
	Duration       string            `json:"duration"`
	Tags           map[string]string `json:"tags"`
}

type Format struct {
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	Tags       map[string]string `json:"tags"`
}

type probeJSON struct {
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}

type Probe struct {
	Stream       Stream
	Format       Format
	Duration     float64
	Matrix       color.Matrix
	MatrixName   string
	MatrixSource string
	Range        Range
}

func ProbeFile(ffprobe, input string) (Probe, error) {
	cmd := exec.Command(ffprobe,
		"-v", "error", "-select_streams", "v:0",
		"-show_entries",
		"stream=index,codec_name,codec_long_name,width,height,pix_fmt,color_range,color_space,color_transfer,color_primaries,duration:stream_tags:format=format_name,duration:format_tags",
		"-of", "json", input,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Probe{}, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var raw probeJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return Probe{}, fmt.Errorf("parse ffprobe JSON: %w", err)
	}
	if len(raw.Streams) == 0 {
		return Probe{}, fmt.Errorf("no video stream: %s", input)
	}
	p := Probe{Stream: raw.Streams[0], Format: raw.Format}
	p.Duration = parseDuration(raw.Format.Duration)
	if p.Duration == 0 {
		p.Duration = parseDuration(p.Stream.Duration)
	}
	p.MatrixName, p.MatrixSource = declaredMatrix(p.Stream, p.Format)
	p.Matrix = color.FromFFprobe(p.MatrixName)
	p.Range = ParseRange(firstNonEmpty(p.Stream.ColorRange, tagValue(p.Stream.Tags, "color_range")))
	if strings.HasPrefix(p.Stream.PixFmt, "yuvj") {
		p.Range = RangeFull
	}
	return p, nil
}

func declaredMatrix(stream Stream, format Format) (string, string) {
	if v := strings.TrimSpace(stream.ColorSpace); v != "" && v != "unknown" {
		return v, "stream.color_space"
	}
	for _, key := range []string{"color_space", "colorspace", "matrix_coefficients"} {
		if v := tagValue(stream.Tags, key); v != "" {
			return v, "stream_tags." + key
		}
	}
	for _, key := range []string{"color_space", "colorspace", "matrix_coefficients"} {
		if v := tagValue(format.Tags, key); v != "" {
			return v, "format_tags." + key
		}
	}
	return "", ""
}

func tagValue(tags map[string]string, key string) string {
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func parseDuration(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
