package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const InputSchema = "hdr-sdr-delivery-input/v1"

type Config struct {
	Schema   string         `json:"schema,omitempty"`
	Project  ProjectConfig  `json:"project"`
	HDR      VersionConfig  `json:"hdr"`
	SDR      VersionConfig  `json:"sdr"`
	Shared   SharedConfig   `json:"shared,omitempty"`
	Timeline TimelineConfig `json:"timeline,omitempty"`
	Cuts     CutsConfig     `json:"cuts,omitempty"`
	Checks   ChecksConfig   `json:"checks,omitempty"`
}

type ProjectConfig struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

type VersionConfig struct {
	Path  string      `json:"path"`
	Label string      `json:"label,omitempty"`
	Color ColorConfig `json:"color,omitempty"`
}

type ColorConfig struct {
	Primaries               string   `json:"primaries,omitempty"`
	Transfer                string   `json:"transfer,omitempty"`
	AllowedTransfers        []string `json:"allowed_transfers,omitempty"`
	Matrix                  string   `json:"matrix,omitempty"`
	Range                   string   `json:"range,omitempty"`
	BitDepth                int      `json:"bit_depth,omitempty"`
	RequireMasteringDisplay *bool    `json:"require_mastering_display,omitempty"`
	RequireContentLight     *bool    `json:"require_content_light,omitempty"`
}

type SharedConfig struct {
	Audio     []SharedAsset `json:"audio,omitempty"`
	Subtitles []SharedAsset `json:"subtitles,omitempty"`
}

type SharedAsset struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  *bool  `json:"default,omitempty"`
}

type TimelineConfig struct {
	DurationToleranceMS int64   `json:"duration_tolerance_ms,omitempty"`
	StartToleranceMS    int64   `json:"start_tolerance_ms,omitempty"`
	FrameRateTolerance  float64 `json:"frame_rate_tolerance,omitempty"`
}

type CutsConfig struct {
	Source      string     `json:"source,omitempty"`
	Path        string     `json:"path,omitempty"`
	Points      []CutPoint `json:"points,omitempty"`
	ToleranceMS int64      `json:"tolerance_ms,omitempty"`
	AllowExtra  bool       `json:"allow_extra,omitempty"`
}

type CutPoint struct {
	ID     string `json:"id"`
	TimeMS int64  `json:"time_ms"`
	Label  string `json:"label,omitempty"`
}

type ChecksConfig struct {
	MediaHashes *bool `json:"media_hashes,omitempty"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Config{}, fmt.Errorf("decode config: unexpected trailing JSON data")
	}
	if cfg.Schema != "" && cfg.Schema != InputSchema {
		return Config{}, fmt.Errorf("unsupported input schema %q; expected %q", cfg.Schema, InputSchema)
	}

	baseDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Config{}, fmt.Errorf("resolve config directory: %w", err)
	}
	cfg.resolvePaths(baseDir)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) resolvePaths(baseDir string) {
	c.HDR.Path = resolvePath(baseDir, c.HDR.Path)
	c.SDR.Path = resolvePath(baseDir, c.SDR.Path)
	c.Cuts.Path = resolvePath(baseDir, c.Cuts.Path)
	for i := range c.Shared.Audio {
		c.Shared.Audio[i].Path = resolvePath(baseDir, c.Shared.Audio[i].Path)
	}
	for i := range c.Shared.Subtitles {
		c.Shared.Subtitles[i].Path = resolvePath(baseDir, c.Shared.Subtitles[i].Path)
	}
}

func resolvePath(baseDir, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.HDR.Path) == "" {
		return errors.New("hdr.path is required")
	}
	if strings.TrimSpace(c.SDR.Path) == "" {
		return errors.New("sdr.path is required")
	}
	if filepath.Clean(c.HDR.Path) == filepath.Clean(c.SDR.Path) {
		return errors.New("hdr.path and sdr.path must be different files")
	}
	if err := validateColorConfig("hdr", c.HDR.Color); err != nil {
		return err
	}
	if err := validateColorConfig("sdr", c.SDR.Color); err != nil {
		return err
	}
	if err := validateTimelineConfig(c.Timeline); err != nil {
		return err
	}
	if err := c.validateCuts(); err != nil {
		return err
	}
	if err := validateAssets("audio", c.Shared.Audio); err != nil {
		return err
	}
	if err := validateAssets("subtitles", c.Shared.Subtitles); err != nil {
		return err
	}
	return validateAssetIDs(c.Shared)
}

func (c Config) validateCuts() error {
	source := strings.ToLower(strings.TrimSpace(c.Cuts.Source))
	switch source {
	case "", "paired", "list":
	default:
		return fmt.Errorf("cuts.source must be paired or list, got %q", c.Cuts.Source)
	}
	if c.Cuts.Path != "" && len(c.Cuts.Points) > 0 {
		return errors.New("cuts.path and cuts.points are mutually exclusive")
	}
	if source == "list" && c.Cuts.Path == "" && len(c.Cuts.Points) == 0 {
		return errors.New("cuts.source=list requires cuts.path or cuts.points")
	}
	if source == "paired" && (c.Cuts.Path != "" || len(c.Cuts.Points) > 0) {
		return errors.New("cuts.source=paired cannot use cuts.path or cuts.points")
	}
	if c.Cuts.ToleranceMS < 0 {
		return errors.New("cuts.tolerance_ms must not be negative")
	}
	if len(c.Cuts.Points) > 0 {
		if err := validateCutPoints(c.Cuts.Points); err != nil {
			return fmt.Errorf("cuts.points: %w", err)
		}
	}
	return nil
}

func validateColorConfig(version string, color ColorConfig) error {
	if color.Transfer != "" && len(color.AllowedTransfers) > 0 {
		return fmt.Errorf("%s.color.transfer and %s.color.allowed_transfers are mutually exclusive", version, version)
	}
	if color.BitDepth < 0 {
		return fmt.Errorf("%s.color.bit_depth must not be negative", version)
	}
	return nil
}

func validateTimelineConfig(timeline TimelineConfig) error {
	if timeline.DurationToleranceMS < 0 {
		return errors.New("timeline.duration_tolerance_ms must not be negative")
	}
	if timeline.StartToleranceMS < 0 {
		return errors.New("timeline.start_tolerance_ms must not be negative")
	}
	if timeline.FrameRateTolerance < 0 {
		return errors.New("timeline.frame_rate_tolerance must not be negative")
	}
	return nil
}

func validateAssets(kind string, assets []SharedAsset) error {
	defaults := 0
	for i, asset := range assets {
		if strings.TrimSpace(asset.ID) == "" {
			return fmt.Errorf("%s[%d].id is required", kind, i)
		}
		if strings.TrimSpace(asset.Path) == "" {
			return fmt.Errorf("%s asset %q path is required", kind, asset.ID)
		}
		if asset.Default != nil && *asset.Default {
			defaults++
		}
	}
	if defaults > 1 {
		return fmt.Errorf("shared.%s has more than one default asset", kind)
	}
	return nil
}

func validateAssetIDs(shared SharedConfig) error {
	seen := make(map[string]string)
	for _, group := range []struct {
		kind   string
		assets []SharedAsset
	}{
		{kind: "audio", assets: shared.Audio},
		{kind: "subtitles", assets: shared.Subtitles},
	} {
		for _, asset := range group.assets {
			if previous, ok := seen[asset.ID]; ok {
				return fmt.Errorf("shared asset id %q is used by both %s and %s", asset.ID, previous, group.kind)
			}
			seen[asset.ID] = group.kind
		}
	}
	return nil
}
