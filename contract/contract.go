// Package contract defines the machine-readable media delivery contract.
//
// A contract is a declarative specification that different platforms can
// version and hand to QA tooling. Every numeric range is inclusive; a zero
// value means "not constrained" so optional rules can be omitted from JSON.
package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// SchemaVersion is bumped whenever the JSON shape changes incompatibly.
const SchemaVersion = "1.0"

// Fraction expresses exact frame rates such as 30000/1001.
type Fraction struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

func (f Fraction) value() (float64, error) {
	if f.Den == 0 {
		return 0, fmt.Errorf("fraction %d/%d has zero denominator", f.Num, f.Den)
	}
	return float64(f.Num) / float64(f.Den), nil
}

// Value returns the frame rate in frames per second.
func (f Fraction) Value() (float64, error) { return f.value() }

// BitrateRule constrains bitrate in bits per second.
type BitrateRule struct {
	MinBPS int64 `json:"minBps,omitempty"`
	MaxBPS int64 `json:"maxBps,omitempty"`
}

// FrameRateRule lists the exact acceptable frame rates.
type FrameRateRule struct {
	Allowed []Fraction `json:"allowed"`
	// Tolerance is the maximum absolute difference in frames per second.
	Tolerance float64 `json:"tolerance,omitempty"`
}

// VideoRule covers codec, frame rate and bitrate constraints.
type VideoRule struct {
	Required  bool           `json:"required"`
	Codecs    []string       `json:"codecs,omitempty"`
	FrameRate *FrameRateRule `json:"frameRate,omitempty"`
	Bitrate   *BitrateRule   `json:"bitrate,omitempty"`
}

// AudioRule covers stream presence, codec, channel layout and bitrate.
type AudioRule struct {
	Required       bool         `json:"required"`
	MinStreams     int          `json:"minStreams,omitempty"`
	MaxStreams     int          `json:"maxStreams,omitempty"`
	Codecs         []string     `json:"codecs,omitempty"`
	ChannelLayouts []string     `json:"channelLayouts,omitempty"`
	Bitrate        *BitrateRule `json:"bitrate,omitempty"`
}

// SubtitleRule covers subtitle presence, codec and language.
type SubtitleRule struct {
	Required  bool     `json:"required"`
	Codecs    []string `json:"codecs,omitempty"`
	Languages []string `json:"languages,omitempty"`
}

// FilenameRule covers naming conventions.
type FilenameRule struct {
	Regex      string   `json:"regex,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
	MaxLength  int      `json:"maxLength,omitempty"`
}

// Contract is the top-level machine-readable delivery specification.
type Contract struct {
	SchemaVersion string       `json:"schemaVersion"`
	Platform      string       `json:"platform"`
	ContractID    string       `json:"contractId"`
	Containers    []string     `json:"containers,omitempty"`
	Video         VideoRule    `json:"video"`
	Audio         AudioRule    `json:"audio"`
	Subtitles     SubtitleRule `json:"subtitles"`
	Filename      FilenameRule `json:"filename"`
}

// Load parses a contract from JSON.
func Load(data []byte) (*Contract, error) {
	var c Contract
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse contract: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks the contract itself so that a malformed specification is
// rejected before any media file is inspected.
func (c *Contract) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %q, want %q", c.SchemaVersion, SchemaVersion)
	}
	if c.Platform == "" {
		return fmt.Errorf("contract platform is empty")
	}
	if c.ContractID == "" {
		return fmt.Errorf("contractId is empty")
	}
	if c.Video.FrameRate != nil {
		if len(c.Video.FrameRate.Allowed) == 0 {
			return fmt.Errorf("video.frameRate.allowed must not be empty")
		}
		for _, f := range c.Video.FrameRate.Allowed {
			if _, err := f.value(); err != nil {
				return fmt.Errorf("video.frameRate: %w", err)
			}
		}
		if c.Video.FrameRate.Tolerance < 0 {
			return fmt.Errorf("video.frameRate.tolerance must be >= 0")
		}
	}
	if c.Video.Bitrate != nil {
		if err := c.Video.Bitrate.validate("video.bitrate"); err != nil {
			return err
		}
	}
	if c.Audio.Bitrate != nil {
		if err := c.Audio.Bitrate.validate("audio.bitrate"); err != nil {
			return err
		}
	}
	if c.Audio.MaxStreams != 0 && c.Audio.MinStreams > c.Audio.MaxStreams {
		return fmt.Errorf("audio.minStreams (%d) exceeds audio.maxStreams (%d)",
			c.Audio.MinStreams, c.Audio.MaxStreams)
	}
	if c.Filename.Regex != "" {
		if _, err := regexp.Compile(c.Filename.Regex); err != nil {
			return fmt.Errorf("filename.regex is invalid: %w", err)
		}
	}
	if c.Filename.MaxLength < 0 {
		return fmt.Errorf("filename.maxLength must be >= 0")
	}
	return nil
}

func (r *BitrateRule) validate(path string) error {
	if r.MinBPS < 0 || r.MaxBPS < 0 {
		return fmt.Errorf("%s bounds must be >= 0", path)
	}
	if r.MaxBPS != 0 && r.MinBPS > r.MaxBPS {
		return fmt.Errorf("%s.minBps (%d) exceeds maxBps (%d)", path, r.MinBPS, r.MaxBPS)
	}
	return nil
}
