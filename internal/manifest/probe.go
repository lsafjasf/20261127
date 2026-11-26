package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type MediaProber interface {
	Probe(ctx context.Context, path string) (ProbeData, error)
	StreamSHA256(ctx context.Context, path string, streamIndex int) (string, error)
}

type ProbeData struct {
	Format   ProbeFormat    `json:"format"`
	Streams  []ProbeStream  `json:"streams"`
	Chapters []ProbeChapter `json:"chapters"`
	Frames   []ProbeFrame   `json:"frames,omitempty"`
}

type ProbeFormat struct {
	Filename   string            `json:"filename"`
	FormatName string            `json:"format_name"`
	StartTime  string            `json:"start_time"`
	Duration   string            `json:"duration"`
	Size       string            `json:"size"`
	BitRate    string            `json:"bit_rate"`
	Tags       map[string]string `json:"tags"`
}

type ProbeStream struct {
	Index            int               `json:"index"`
	CodecType        string            `json:"codec_type"`
	CodecName        string            `json:"codec_name"`
	Profile          string            `json:"profile"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	PixelFormat      string            `json:"pix_fmt"`
	BitsPerRawSample string            `json:"bits_per_raw_sample"`
	ColorRange       string            `json:"color_range"`
	ColorSpace       string            `json:"color_space"`
	ColorTransfer    string            `json:"color_transfer"`
	ColorPrimaries   string            `json:"color_primaries"`
	Duration         string            `json:"duration"`
	StartTime        string            `json:"start_time"`
	AverageFrameRate string            `json:"avg_frame_rate"`
	RealFrameRate    string            `json:"r_frame_rate"`
	TimeBase         string            `json:"time_base"`
	NBFrames         string            `json:"nb_frames"`
	NBReadFrames     string            `json:"nb_read_frames"`
	Channels         int               `json:"channels"`
	ChannelLayout    string            `json:"channel_layout"`
	SampleRate       string            `json:"sample_rate"`
	Disposition      map[string]int    `json:"disposition"`
	Tags             map[string]string `json:"tags"`
	SideDataList     []ProbeSideData   `json:"side_data_list"`
}

type ProbeFrame struct {
	ColorRange     string          `json:"color_range"`
	ColorSpace     string          `json:"color_space"`
	ColorTransfer  string          `json:"color_transfer"`
	ColorPrimaries string          `json:"color_primaries"`
	PixelFormat    string          `json:"pix_fmt"`
	SideDataList   []ProbeSideData `json:"side_data_list"`
}

type ProbeSideData struct {
	Type         string `json:"side_data_type"`
	RedX         string `json:"red_x"`
	RedY         string `json:"red_y"`
	GreenX       string `json:"green_x"`
	GreenY       string `json:"green_y"`
	BlueX        string `json:"blue_x"`
	BlueY        string `json:"blue_y"`
	WhitePointX  string `json:"white_point_x"`
	WhitePointY  string `json:"white_point_y"`
	MinLuminance string `json:"min_luminance"`
	MaxLuminance string `json:"max_luminance"`
	MaxContent   int    `json:"max_content"`
	MaxAverage   int    `json:"max_average"`
}

type ProbeChapter struct {
	ID        JSONString        `json:"id"`
	StartTime string            `json:"start_time"`
	EndTime   string            `json:"end_time"`
	Tags      map[string]string `json:"tags"`
}

type JSONString string

func (value *JSONString) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*value = ""
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		*value = JSONString(decoded)
		return nil
	}
	*value = JSONString(trimmed)
	return nil
}

func (p ProbeData) StreamsOfType(kind string) []ProbeStream {
	var streams []ProbeStream
	for _, stream := range p.Streams {
		if strings.EqualFold(stream.CodecType, kind) {
			streams = append(streams, stream)
		}
	}
	return streams
}

func (p ProbeData) FirstVideo() (ProbeStream, bool) {
	for _, stream := range p.Streams {
		if strings.EqualFold(stream.CodecType, "video") {
			return stream, true
		}
	}
	return ProbeStream{}, false
}

func (p ProbeData) FirstStreamOfType(kind string) (ProbeStream, bool) {
	for _, stream := range p.Streams {
		if strings.EqualFold(stream.CodecType, kind) {
			return stream, true
		}
	}
	return ProbeStream{}, false
}

func (s ProbeStream) AllSideData(frameSideData []ProbeSideData) []ProbeSideData {
	all := make([]ProbeSideData, 0, len(s.SideDataList)+len(frameSideData))
	all = append(all, s.SideDataList...)
	all = append(all, frameSideData...)
	return all
}

func (s ProbeStream) IsDefault() bool {
	return s.Disposition["default"] == 1
}

func (s ProbeStream) IsForced() bool {
	return s.Disposition["forced"] == 1
}

func (s ProbeStream) Tag(name string) string {
	for key, value := range s.Tags {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func HasSideData(items []ProbeSideData, needle string) bool {
	needle = strings.ToLower(needle)
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Type), needle) {
			return true
		}
	}
	return false
}

func FindSideData(items []ProbeSideData, needle string) (ProbeSideData, bool) {
	needle = strings.ToLower(needle)
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Type), needle) {
			return item, true
		}
	}
	return ProbeSideData{}, false
}

func SecondsToMS(value string) (int64, bool) {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, false
	}
	return int64(math.Round(seconds * 1000)), true
}

func ParseRational(value string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 {
		return 0, false
	}
	numerator, err1 := strconv.ParseFloat(parts[0], 64)
	denominator, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || numerator <= 0 || denominator <= 0 {
		return 0, false
	}
	return numerator / denominator, true
}

func ParsePositiveInt(value string) (int, bool) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

func ParsePositiveInt64(value string) (int64, bool) {
	number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

var trailingBitDepth = regexp.MustCompile(`([0-9]{1,2})(?:le|be)$`)

func PixelBitDepth(stream ProbeStream) int {
	if depth, ok := ParsePositiveInt(stream.BitsPerRawSample); ok {
		return depth
	}
	format := strings.ToLower(stream.PixelFormat)
	if match := trailingBitDepth.FindStringSubmatch(format); len(match) == 2 {
		depth, _ := strconv.Atoi(match[1])
		return depth
	}
	if format != "" {
		return 8
	}
	return 0
}

type ExecProbe struct {
	FFprobe string
	FFmpeg  string
}

func NewExecProbe(ffprobePath, ffmpegPath string) *ExecProbe {
	return &ExecProbe{FFprobe: ffprobePath, FFmpeg: ffmpegPath}
}

func (e *ExecProbe) Probe(ctx context.Context, path string) (ProbeData, error) {
	args := []string{
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		path,
	}
	output, err := runCommand(ctx, e.FFprobe, args...)
	if err != nil {
		return ProbeData{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var probe ProbeData
	if err := json.Unmarshal(output, &probe); err != nil {
		return ProbeData{}, fmt.Errorf("decode ffprobe output for %s: %w", path, err)
	}
	if _, ok := probe.FirstVideo(); !ok {
		return probe, nil
	}

	countArgs := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-count_frames",
		"-show_entries", "stream=nb_read_frames",
		"-print_format", "json",
		path,
	}
	countOutput, err := runCommand(ctx, e.FFprobe, countArgs...)
	if err != nil {
		return ProbeData{}, fmt.Errorf("ffprobe frame count %s: %w", path, err)
	}
	var counted ProbeData
	if err := json.Unmarshal(countOutput, &counted); err != nil {
		return ProbeData{}, fmt.Errorf("decode ffprobe frame count for %s: %w", path, err)
	}
	if len(counted.Streams) > 0 {
		for i := range probe.Streams {
			if strings.EqualFold(probe.Streams[i].CodecType, "video") {
				probe.Streams[i].NBReadFrames = counted.Streams[0].NBReadFrames
				break
			}
		}
	}

	frameArgs := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-read_intervals", "%+#1",
		"-show_frames",
		"-show_entries", "frame=color_range,color_space,color_transfer,color_primaries,pix_fmt,side_data_list",
		"-print_format", "json",
		path,
	}
	frameOutput, err := runCommand(ctx, e.FFprobe, frameArgs...)
	if err != nil {
		return ProbeData{}, fmt.Errorf("ffprobe first frame %s: %w", path, err)
	}
	var frames ProbeData
	if err := json.Unmarshal(frameOutput, &frames); err != nil {
		return ProbeData{}, fmt.Errorf("decode ffprobe frame output for %s: %w", path, err)
	}
	probe.Frames = frames.Frames
	return probe, nil
}

func (e *ExecProbe) StreamSHA256(ctx context.Context, path string, streamIndex int) (string, error) {
	args := []string{
		"-v", "error",
		"-nostdin",
		"-i", path,
		"-map", fmt.Sprintf("0:%d", streamIndex),
		"-c", "copy",
		"-f", "streamhash",
		"-hash", "sha256",
		"-",
	}
	output, err := runCommand(ctx, e.FFmpeg, args...)
	if err != nil {
		return "", fmt.Errorf("stream hash %s#%d: %w", path, streamIndex, err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "SHA256=") {
			continue
		}
		hash := strings.TrimSpace(strings.SplitN(line, "SHA256=", 2)[1])
		if hash != "" {
			return hash, nil
		}
	}
	return "", fmt.Errorf("stream hash %s#%d: no SHA256 output", path, streamIndex)
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("empty executable path")
	}
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		detail := strings.TrimSpace(string(exitError.Stderr))
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
	}
	return nil, err
}
