package rangecheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type ffProbeJSON struct {
	Streams []struct {
		Index        int    `json:"index"`
		CodecName    string `json:"codec_name"`
		CodecType    string `json:"codec_type"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		PixelFormat  string `json:"pix_fmt"`
		ColorRange   string `json:"color_range"`
		ColorSpace   string `json:"color_space"`
		RFrameRate   string `json:"r_frame_rate"`
		AvgFrameRate string `json:"avg_frame_rate"`
		NbFrames     string `json:"nb_frames"`
		Duration     string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
	} `json:"format"`
}

type ffFrameList struct {
	Frames []struct {
		PTSTime    string `json:"pts_time"`
		BestEffort string `json:"best_effort_timestamp_time"`
		PktPTSTime string `json:"pkt_pts_time"`
	} `json:"frames"`
}

// ParseRational parses FFprobe's "30000/1001" or integer frame rate.
func ParseRational(value string) Rational {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" || value == "N/A" {
		return Rational{}
	}
	parts := strings.Split(value, "/")
	if len(parts) == 1 {
		n, _ := strconv.ParseInt(parts[0], 10, 64)
		if n == 0 {
			return Rational{}
		}
		return Rational{Num: n, Den: 1}
	}
	n, err1 := strconv.ParseInt(parts[0], 10, 64)
	d, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return Rational{}
	}
	return Rational{Num: n, Den: d}
}

// ProbeVideo reads stream metadata using ffprobe. It is optional for callers
// that already know width, height, and frame rate.
func ProbeVideo(path, ffprobeBin string) (Metadata, []float64, error) {
	if ffprobeBin == "" {
		ffprobeBin = "ffprobe"
	}
	cmd := exec.Command(ffprobeBin, "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Metadata{}, nil, fmt.Errorf("ffprobe metadata failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var probe ffProbeJSON
	if err := json.Unmarshal(out, &probe); err != nil {
		return Metadata{}, nil, fmt.Errorf("parse ffprobe metadata: %w", err)
	}
	var meta Metadata
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			meta.Codec = stream.CodecName
			meta.Width = stream.Width
			meta.Height = stream.Height
			meta.PixelFormat = stream.PixelFormat
			meta.ColorRange = stream.ColorRange
			meta.ColorSpace = stream.ColorSpace
			meta.FrameRate = ParseRational(stream.RFrameRate)
			meta.AverageRate = ParseRational(stream.AvgFrameRate)
			if n, _ := strconv.Atoi(stream.NbFrames); n > 0 {
				meta.NbFrames = n
			}
			if d, _ := strconv.ParseFloat(firstNonEmpty(stream.Duration, probe.Format.Duration), 64); d > 0 {
				meta.Duration = d
			} else if d, _ := strconv.ParseFloat(probe.Format.Duration, 64); d > 0 {
				meta.Duration = d
			}
			break
		}
	}
	meta.Format = probe.Format.FormatName
	framePTS, err := probeFramePTS(path, ffprobeBin)
	if err != nil {
		return meta, nil, nil
	}
	return meta, framePTS, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" && v != "N/A" {
			return v
		}
	}
	return ""
}

func probeFramePTS(path, ffprobeBin string) ([]float64, error) {
	cmd := exec.Command(ffprobeBin, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "frame=pts_time,best_effort_timestamp_time,pkt_pts_time",
		"-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var listing ffFrameList
	if err := json.Unmarshal(out, &listing); err != nil {
		return nil, err
	}
	var pts []float64
	for _, frame := range listing.Frames {
		raw := firstNonEmpty(frame.BestEffort, frame.PTSTime, frame.PktPTSTime)
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		pts = append(pts, v)
	}
	return pts, nil
}

func processVideo(path string, analyzer *Analyzer, options ProcessOptions, externalPTS []float64) error {
	ffmpegBin := options.FFmpegPath
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}
	settings := options.Settings
	if settings.Width <= 0 {
		settings.Width = analyzer.Report.Metadata.Width
		settings.Height = analyzer.Report.Metadata.Height
	}
	if settings.FPS.Num == 0 {
		settings.FPS = analyzer.Report.Metadata.FrameRate
	}
	settings.PixelFormat = FormatYUV444P
	if settings.Range == "" {
		settings.Range = RangeTV
	}
	analyzer.settings.PixelFormat = FormatYUV444P
	analyzer.Report.Settings.PixelFormat = FormatYUV444P
	analyzer.Report.Metadata.Decoder = "ffmpeg rawvideo/yuv444p (Y codes analyzed before RGB conversion)"

	pts := externalPTS
	if len(pts) == 0 && options.PTSFile == "" {
		if _, probed, err := ProbeVideo(path, options.FFprobePath); err == nil {
			pts = probed
		}
	}

	cmd := exec.Command(ffmpegBin, "-hide_banner", "-nostdin", "-v", "error",
		"-i", path, "-map", "0:v:0", "-an", "-sn", "-dn",
		"-f", "rawvideo", "-pix_fmt", "yuv444p", "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	frameSize := settings.Width * settings.Height * 3
	if frameSize <= 0 {
		_ = cmd.Process.Kill()
		return fmt.Errorf("video dimensions are unknown; pass --width/--height or ensure ffprobe is available")
	}
	buffer := make([]byte, frameSize)
	frameIndex := 0
	readErr := error(nil)
	for options.MaxFrames == 0 || frameIndex < options.MaxFrames {
		_, err := io.ReadFull(stdout, buffer)
		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF {
			readErr = fmt.Errorf("ffmpeg returned a partial raw frame at frame %d", frameIndex)
			break
		}
		if err != nil {
			readErr = err
			break
		}
		frame, err := rawFrame(buffer, frameIndex, ptsForFrame(frameIndex, pts, settings.FPS), settings)
		if err != nil {
			readErr = err
			break
		}
		if _, err := addFrame(analyzer, frame, options); err != nil {
			readErr = err
			break
		}
		frameIndex++
	}

	if readErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return readErr
	}
	limitReached := options.MaxFrames > 0 && frameIndex >= options.MaxFrames
	if limitReached {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	} else {
		waitErr := cmd.Wait()
		if waitErr != nil {
			return fmt.Errorf("ffmpeg decode failed: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
	}
	if frameIndex == 0 {
		return fmt.Errorf("ffmpeg produced no video frames: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}
