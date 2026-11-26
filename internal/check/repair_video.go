package check

import (
	"os"
	"strings"

	"proalpha/internal/mediaio"
	"proalpha/internal/mediameta"
)

func repairVideo(sourceMeta *mediameta.FileMetadata, outputPath string) error {
	return mediaio.RunFFmpeg(repairArguments(sourceMeta, outputPath)...)
}

func writeRepairScript(sourceMeta *mediameta.FileMetadata, scriptPath, videoPath string) error {
	args := repairArguments(sourceMeta, videoPath)
	var script strings.Builder
	script.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")
	script.WriteString("ffmpeg")
	for _, arg := range args {
		script.WriteString(" ")
		script.WriteString(shellQuote(arg))
	}
	script.WriteString("\n")
	if err := os.WriteFile(scriptPath, []byte(script.String()), 0o755); err != nil {
		return err
	}
	return nil
}

func repairArguments(sourceMeta *mediameta.FileMetadata, outputPath string) []string {
	primaries, transfer, matrix, rangeName := ReferenceColor(sourceMeta)
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", sourceMeta.Path,
		"-map", "0:v:0", "-an",
		"-c:v", "prores_ks", "-profile:v", "4444",
		"-pix_fmt", "yuva444p10le", "-alpha_bits", "16",
		"-color_primaries", primaries,
		"-color_trc", transfer,
		"-colorspace", matrix,
	}
	if rangeName != "" {
		args = append(args, "-color_range", rangeName)
	}
	args = append(args, outputPath)
	return args
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\r\n\"'\\$`!*?[]{}<>()|&;#") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\"'\"'`) + "'"
}
