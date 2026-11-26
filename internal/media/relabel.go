package media

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"colorfix/internal/color"
	"colorfix/internal/video"
)

type RelabelResult struct {
	Path             string `json:"path"`
	BitstreamUpdated bool   `json:"bitstream_updated"`
	VerifiedMatrix   string `json:"verified_matrix,omitempty"`
}

func Relabel(ffmpeg, input, output string, matrix color.Matrix, probe video.Probe, overwrite bool) (RelabelResult, error) {
	if !overwrite {
		if _, err := os.Stat(output); err == nil {
			return RelabelResult{}, fmt.Errorf("output exists: %s", output)
		}
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", input, "-map", "0", "-c", "copy", "-map_metadata", "0"}
	bsf, ok := matrixFilter(probe.Stream.CodecName, matrix)
	if ok {
		args = append(args, "-bsf:v", bsf)
	}
	args = append(args, "-colorspace", matrix.FFprobeName())
	if strings.HasPrefix(probe.Stream.PixFmt, "yuvj") {
		args = append(args, "-color_range", "pc")
	}
	if strings.EqualFold(filepath.Ext(output), ".mp4") {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, output)
	cmd := exec.Command(ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return RelabelResult{}, fmt.Errorf("ffmpeg relabel: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return RelabelResult{Path: output, BitstreamUpdated: ok}, nil
}

func matrixFilter(codec string, matrix color.Matrix) (string, bool) {
	value := 5
	if matrix == color.BT709 {
		value = 1
	}
	switch codec {
	case "h264":
		return fmt.Sprintf("h264_metadata=matrix_coefficients=%d", value), true
	case "hevc":
		return fmt.Sprintf("hevc_metadata=matrix_coefficients=%d", value), true
	case "mpeg2video":
		return fmt.Sprintf("mpeg2_metadata=matrix_coefficients=%d", value), true
	case "av1":
		return fmt.Sprintf("av1_metadata=matrix_coefficients=%d", value), true
	case "vp9":
		value = 1
		if matrix == color.BT709 {
			value = 2
		}
		return fmt.Sprintf("vp9_metadata=color_space=%d", value), true
	default:
		return "", false
	}
}
