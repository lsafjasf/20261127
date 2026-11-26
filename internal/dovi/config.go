package dovi

import (
	"fmt"
)

// Config is the DOVIDecoderConfigurationRecord carried by dvcC or dvvC.
type Config struct {
	VersionMajor        int
	VersionMinor        int
	Profile             int
	Level               int
	RPUPresent          bool
	ELPresent           bool
	BLPresent           bool
	CompatibilityID     int
	MetadataCompression int
	FeatureFlags        []bool
	RawLength           int
}

// ParseConfig accepts a configuration record payload, without the box header.
// Older files may contain only the first five bytes; compatibility and
// compression then have their reserved default of zero.
func ParseConfig(payload []byte) (*Config, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("Dolby Vision configuration is %d bytes; need at least 4", len(payload))
	}
	cfg := &Config{
		VersionMajor: int(payload[0]),
		VersionMinor: int(payload[1]),
		RawLength:    len(payload),
	}
	v := uint16(payload[2])<<8 | uint16(payload[3])
	cfg.Profile = int((v >> 9) & 0x7f)
	cfg.Level = int((v >> 3) & 0x3f)
	cfg.RPUPresent = v&(1<<2) != 0
	cfg.ELPresent = v&(1<<1) != 0
	cfg.BLPresent = v&1 != 0

	if len(payload) >= 5 {
		cfg.CompatibilityID = int(payload[4] >> 4)
		cfg.MetadataCompression = int((payload[4] >> 2) & 0x03)
		cfg.FeatureFlags = make([]bool, 10)
		cfg.FeatureFlags[0] = payload[4]&(1<<1) != 0
		cfg.FeatureFlags[1] = payload[4]&1 != 0
		if len(payload) >= 6 {
			for i := 0; i < 8; i++ {
				cfg.FeatureFlags[i+2] = payload[5]&(1<<(7-i)) != 0
			}
		}
	}
	return cfg, nil
}

func (c *Config) ExpectedBox() string {
	switch {
	case c == nil:
		return ""
	case c.Profile <= 7, c.Profile == 20:
		return "dvcC"
	case c.Profile >= 8 && c.Profile <= 10:
		return "dvvC"
	default:
		return ""
	}
}

func (c *Config) CompressedMetadata() bool {
	return c != nil && c.MetadataCompression != 0
}

func CompatibilityFormat(id int) string {
	switch id {
	case 0:
		return "none/IPTPQc2-only"
	case 1:
		return "HDR10/PQ10"
	case 2:
		return "SDR/BT.1886"
	case 3:
		return "HLG/BT.709-in-BT.2020"
	case 4:
		return "HLG/BT.2020"
	case 5:
		return "SDR/BT.2020"
	case 6:
		return "UHD-Blu-ray HDR10"
	default:
		return "reserved"
	}
}

func ProfileName(profile int) string {
	switch profile {
	case 4:
		return "Profile 4 (HEVC BL+EL+RPU)"
	case 5:
		return "Profile 5 (HEVC RPU-only, IPTPQc2 base)"
	case 7:
		return "Profile 7 (HEVC BL+EL+RPU, UHD Blu-ray)"
	case 8:
		return "Profile 8 (HEVC RPU-only, compatible base)"
	case 9:
		return "Profile 9 (AVC RPU-only, compatible base)"
	case 10:
		return "Profile 10 (AV1 RPU-only, compatible base)"
	case 20:
		return "Profile 20 (HEVC Multiview)"
	default:
		return fmt.Sprintf("Profile %d (unknown/reserved)", profile)
	}
}

func IsDualLayerProfile(profile int) bool {
	return profile == 4 || profile == 7
}

func IsSingleLayerProfile(profile int) bool {
	return profile == 5 || profile == 8 || profile == 9 || profile == 10
}

type LevelLimit struct {
	MaxPixelsPerSecond uint64
	MaxWidth           int
	MainMbps           int
	HighMbps           int
}

// Limits follow the Dolby Vision level table. Level 13 is valid, but values
// above it are not assigned by the specification.
func Limits(level int) (LevelLimit, bool) {
	limits := [...]LevelLimit{
		{},
		{1280 * 720 * 24, 1280, 20, 50},
		{1280 * 720 * 30, 1280, 20, 50},
		{1920 * 1080 * 24, 1920, 20, 70},
		{1920 * 1080 * 30, 2560, 20, 70},
		{1920 * 1080 * 60, 3840, 20, 70},
		{3840 * 2160 * 24, 3840, 25, 130},
		{3840 * 2160 * 30, 3840, 25, 130},
		{3840 * 2160 * 48, 3840, 40, 130},
		{3840 * 2160 * 60, 3840, 40, 130},
		{3840 * 2160 * 120, 3840, 60, 240},
		{3840 * 2160 * 120, 7680, 60, 240},
		{7680 * 4320 * 60, 7680, 120, 450},
		{7680 * 4320 * 120, 7680, 240, 800},
	}
	if level < 1 || level >= len(limits) {
		return LevelLimit{}, false
	}
	return limits[level], true
}
