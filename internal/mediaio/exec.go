package mediaio

import (
	"bytes"
	"fmt"
	"os/exec"
)

func RunFFmpeg(args ...string) error {
	bin, err := FFmpegPath()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd := exec.Command(bin, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg %v: %w: %s", args, err, stderr.String())
	}
	return nil
}
