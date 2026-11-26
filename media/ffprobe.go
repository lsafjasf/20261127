package media

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ffprobeJSON is the subset of ffprobe output we consume.
type ffprobeJSON struct {
	Format struct {
		FormatName string `json:"format_name"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index         int    `json:"index"`
		CodecType     string `json:"codec_type"`
		CodecName     string `json:"codec_name"`
		AvgFrameRate  string `json:"avg_frame_rate"`
		RFrameRate    string `json:"r_frame_rate"`
		BitRate       string `json:"bit_rate"`
		Channels      int    `json:"channels"`
		ChannelLayout string `json:"channel_layout"`
		Tags          struct {
			Language string `json:"language"`
		} `json:"tags"`
	} `json:"streams"`
}

// ProbeWithFFProbe runs the ffprobe binary and converts its output into the
// tool-neutral Probe model.
func ProbeWithFFProbe(bin, path string) (*Probe, error) {
	out, err := exec.Command(bin, "-v", "error", "-show_format",
		"-show_streams", "-of", "json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", bin, err)
	}
	return ParseFFProbeJSON(path, out)
}

// ParseFFProbeJSON converts raw ffprobe JSON. It is split from the command
// invocation so adapter behavior can be unit tested without a binary.
func ParseFFProbeJSON(path string, raw []byte) (*Probe, error) {
	var parsed ffprobeJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse ffprobe json: %w", err)
	}

	probe := &Probe{Path: path}
	if names := splitAliases(parsed.Format.FormatName); len(names) > 0 {
		probe.Container = names[0]
		probe.ContainerAliases = names
	}

	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			probe.Video = append(probe.Video, VideoStream{
				Codec:     stream.CodecName,
				FrameRate: parseFraction(firstNonEmpty(stream.AvgFrameRate, stream.RFrameRate)),
				BPS:       parseBPS(stream.BitRate),
			})
		case "audio":
			probe.Audio = append(probe.Audio, AudioStream{
				Codec:         stream.CodecName,
				ChannelLayout: stream.ChannelLayout,
				Channels:      stream.Channels,
				BPS:           parseBPS(stream.BitRate),
			})
		case "subtitle":
			probe.Subtitles = append(probe.Subtitles, SubtitleStream{
				Codec:    stream.CodecName,
				Language: stream.Tags.Language,
			})
		}
	}
	return probe, nil
}

func splitAliases(value string) []string {
	var aliases []string
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			aliases = append(aliases, name)
		}
	}
	return aliases
}

func parseFraction(value string) Fraction {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return Fraction{}
	}
	num, err1 := strconv.Atoi(parts[0])
	den, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || den == 0 {
		return Fraction{}
	}
	return Fraction{Num: num, Den: den}
}

func parseBPS(value string) int64 {
	if value == "" || value == "N/A" {
		return 0
	}
	bps, err := strconv.ParseInt(value, 10, 64)
	if err != nil || bps <= 0 {
		return 0
	}
	return bps
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" && value != "0/0" {
			return value
		}
	}
	return ""
}
