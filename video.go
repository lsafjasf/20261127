package tearcheck

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// VideoInfo contains the small ffprobe subset needed by the analyzer.
type VideoInfo struct {
	Codec    string
	Width    int
	Height   int
	FPS      float64
	Duration float64
}

type ffprobeJSON struct {
	Streams []struct {
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		RFrameRate   string `json:"r_frame_rate"`
	} `json:"streams"`
	Format *struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// ProbeVideo calls ffprobe and returns codec/geometry/frame-rate metadata.
func ProbeVideo(ctx context.Context, path, ffprobeBin string) (VideoInfo, error) {
	if ffprobeBin == "" {
		ffprobeBin = "ffprobe"
	}
	cmd := exec.CommandContext(ctx, ffprobeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height,avg_frame_rate,r_frame_rate",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	)
	output, err := cmd.Output()
	if err != nil {
		return VideoInfo{}, fmt.Errorf("ffprobe failed: %w", err)
	}
	var parsed ffprobeJSON
	if err := json.Unmarshal(output, &parsed); err != nil {
		return VideoInfo{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	if len(parsed.Streams) == 0 {
		return VideoInfo{}, errors.New("no video stream found")
	}
	stream := parsed.Streams[0]
	info := VideoInfo{Codec: stream.CodecName, Width: stream.Width, Height: stream.Height}
	info.FPS = parseFrameRate(stream.AvgFrameRate)
	if info.FPS <= 0 {
		info.FPS = parseFrameRate(stream.RFrameRate)
	}
	if parsed.Format != nil {
		info.Duration, _ = strconv.ParseFloat(parsed.Format.Duration, 64)
	}
	return info, nil
}

func parseFrameRate(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0
	}
	numerator, err1 := strconv.ParseFloat(parts[0], 64)
	denominator, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || denominator == 0 {
		return 0
	}
	return numerator / denominator
}

// ProbeFramePTS returns per-frame presentation timestamps when available.
func ProbeFramePTS(ctx context.Context, path, ffprobeBin string) ([]float64, error) {
	if ffprobeBin == "" {
		ffprobeBin = "ffprobe"
	}
	cmd := exec.CommandContext(ctx, ffprobeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "frame=pts_time",
		"-of", "csv=p=0",
		path,
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe frame pts failed: %w", err)
	}
	var points []float64
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "N/A") {
			continue
		}
		value, err := strconv.ParseFloat(line, 64)
		if err == nil {
			points = append(points, value)
		}
	}
	return points, nil
}

// ReadMetadataFile reads the optional capture/codec/content side-car JSON.
func ReadMetadataFile(path string) (Metadata, error) {
	var metadata Metadata
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	return metadata, nil
}

var mbErrorPattern = regexp.MustCompile(`error while decoding MB\s+(\d+)\s+(\d+)`)

func scanFFmpegLogs(reader io.Reader, events *[]CodecEvent, mutex *sync.Mutex) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		event := CodecEvent{Confidence: 0.72, Message: line}
		if match := mbErrorPattern.FindStringSubmatch(line); match != nil {
			mbY, _ := strconv.Atoi(match[2])
			event.Y = mbY * 16
			event.Confidence = 0.96
		} else if strings.Contains(line, "concealing") {
			event.Confidence = 0.78
		} else if strings.Contains(line, "Invalid data found") ||
			strings.Contains(line, "error while decoding") {
			event.Confidence = 0.88
		} else {
			continue
		}
		mutex.Lock()
		*events = append(*events, event)
		mutex.Unlock()
	}
}

// VideoOptions controls optional external decoder execution.
type VideoOptions struct {
	FFmpegBin  string
	FFprobeBin string
	MaxFrames  int
	Metadata   Metadata
	OnWarning  func(string)
}

// AnalyzeVideo probes and decodes a video through ffmpeg. The core detector
// remains pure Go; only demuxing/decoding is delegated to the system binary.
func AnalyzeVideo(ctx context.Context, path string, options VideoOptions) (*Report, error) {
	info, err := ProbeVideo(ctx, path, options.FFprobeBin)
	if err != nil {
		return nil, err
	}
	framePTS, ptsErr := ProbeFramePTS(ctx, path, options.FFprobeBin)
	if ptsErr != nil && options.OnWarning != nil {
		options.OnWarning(ptsErr.Error())
	}
	if info.Width <= 0 || info.Height <= 0 {
		return nil, errors.New("ffprobe returned invalid frame dimensions")
	}
	metadata := options.Metadata
	if metadata.Codec == "" {
		metadata.Codec = info.Codec
	}
	if metadata.Width == 0 {
		metadata.Width = info.Width
	}
	if metadata.Height == 0 {
		metadata.Height = info.Height
	}

	analyzer := NewAnalyzer(metadata)
	analyzer.BlockSizes = DefaultBlockSizes(info.Codec)

	if options.FFmpegBin == "" {
		options.FFmpegBin = "ffmpeg"
	}
	args := []string{"-hide_banner", "-nostdin", "-i", path, "-map", "0:v:0"}
	if options.MaxFrames > 0 {
		args = append(args, "-frames:v", strconv.Itoa(options.MaxFrames))
	}
	args = append(args, "-pix_fmt", "rgba", "-f", "rawvideo", "-")
	cmd := exec.CommandContext(ctx, options.FFmpegBin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	var logEvents []CodecEvent
	var logMutex sync.Mutex
	var logsDone sync.WaitGroup
	logsDone.Add(1)
	go func() {
		defer logsDone.Done()
		scanFFmpegLogs(stderr, &logEvents, &logMutex)
	}()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	frameIndex := 0
	readErr := error(nil)
	for {
		rgba := image.NewNRGBA(image.Rect(0, 0, info.Width, info.Height))
		_, err := io.ReadFull(stdout, rgba.Pix)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) && frameIndex == 0 {
			break
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				if options.OnWarning != nil {
					options.OnWarning("truncated final raw video frame")
				}
				break
			}
			readErr = err
			break
		}
		pts := 0.0
		switch {
		case frameIndex < len(framePTS):
			pts = framePTS[frameIndex]
		case info.FPS > 0:
			pts = float64(frameIndex) / info.FPS
		}
		analyzer.PushImage(rgba, pts)
		frameIndex++
	}

	waitErr := cmd.Wait()
	logsDone.Wait()
	logMutex.Lock()
	analyzer.Metadata.CodecEvents = append(analyzer.Metadata.CodecEvents, logEvents...)
	logMutex.Unlock()

	if readErr != nil && !errors.Is(readErr, context.Canceled) {
		return nil, fmt.Errorf("read raw video: %w", readErr)
	}
	if waitErr != nil && frameIndex == 0 {
		return nil, fmt.Errorf("ffmpeg decode failed: %w", waitErr)
	}
	report := analyzer.Finish()
	if waitErr != nil && options.OnWarning != nil {
		options.OnWarning("ffmpeg exited with warnings after producing frames: " + waitErr.Error())
	}
	return report, nil
}
