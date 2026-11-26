package mediameta

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrFFProbeNotFound = errors.New("ffprobe not found in PATH")

func FFProbePath() (string, error) {
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", ErrFFProbeNotFound
	}
	return path, nil
}

func Probe(path string) (*Stream, error) {
	bin, err := FFProbePath()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "-v", "error", "-show_streams", "-of", "json", path)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("ffprobe failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	var report ProbeReport
	if err := json.Unmarshal(output, &report); err != nil {
		return nil, fmt.Errorf("decode ffprobe json: %w", err)
	}
	for _, stream := range report.Streams {
		if stream.CodecType == "video" {
			streamCopy := stream
			return &streamCopy, nil
		}
	}
	return nil, errors.New("no video stream found by ffprobe")
}

func Inspect(path string) (*FileMetadata, error) {
	info, err := parseMOVFile(path)
	if err != nil {
		return nil, err
	}
	meta := &FileMetadata{Path: path, MOV: info}
	if size, err := fileSize(path); err == nil {
		meta.Size = size
	}
	if stream, err := Probe(path); err == nil {
		meta.FFProbeAvailable = true
		meta.FFProbe = stream
	} else if !errors.Is(err, ErrFFProbeNotFound) {
		return nil, err
	}
	return meta, nil
}
