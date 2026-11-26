// Package media contains the tool-neutral probe model fed to the validator.
//
// Probe intentionally only carries information the contract can reason about.
// Adapters (for example the ffprobe adapter) translate vendor-specific
// output into this shape. Unknown information is represented by zero values,
// and the validator distinguishes "field absent" from "field measured".
package media

import "fmt"

// Fraction mirrors contract.Fraction without importing the contract package.
type Fraction struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

// FPS returns the frame rate as frames per second.
func (f Fraction) FPS() (float64, error) {
	if f.Den == 0 {
		return 0, fmt.Errorf("frame rate %d/%d has zero denominator", f.Num, f.Den)
	}
	return float64(f.Num) / float64(f.Den), nil
}

// VideoStream describes a single video stream.
type VideoStream struct {
	Codec     string   `json:"codec"`
	FrameRate Fraction `json:"frameRate"`
	// BPS <= 0 means the probe could not determine bitrate.
	BPS int64 `json:"bps,omitempty"`
}

// AudioStream describes a single audio stream.
type AudioStream struct {
	Codec         string `json:"codec"`
	ChannelLayout string `json:"channelLayout"`
	Channels      int    `json:"channels"`
	BPS           int64  `json:"bps,omitempty"`
}

// SubtitleStream describes a single subtitle stream.
type SubtitleStream struct {
	Codec    string `json:"codec"`
	Language string `json:"language"`
}

// Probe is the full tool-neutral description of one media file.
type Probe struct {
	Path string `json:"path"`
	// Container holds every format alias reported by the probe, e.g.
	// ["mov","mp4"] for an MP4 file. A contract matches if it lists any.
	Container        string           `json:"container"`
	ContainerAliases []string         `json:"containerAliases,omitempty"`
	Video            []VideoStream    `json:"video"`
	Audio            []AudioStream    `json:"audio"`
	Subtitles        []SubtitleStream `json:"subtitles"`
}
