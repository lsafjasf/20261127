package audio

import (
	"fmt"
	"strings"
)

const (
	SpeakerFrontLeft   uint32 = 0x0001
	SpeakerFrontRight  uint32 = 0x0002
	SpeakerFrontCenter uint32 = 0x0004
	SpeakerLFE         uint32 = 0x0008
	SpeakerBackLeft    uint32 = 0x0010
	SpeakerBackRight   uint32 = 0x0020
	SpeakerSideLeft    uint32 = 0x0200
	SpeakerSideRight   uint32 = 0x0400
)

var speakerOrder = []struct {
	bit  uint32
	name string
}{
	{SpeakerFrontLeft, "FL"},
	{SpeakerFrontRight, "FR"},
	{SpeakerFrontCenter, "FC"},
	{SpeakerLFE, "LFE"},
	{SpeakerBackLeft, "BL"},
	{SpeakerBackRight, "BR"},
	{0x0040, "FLC"},
	{0x0080, "FRC"},
	{0x0100, "BC"},
	{SpeakerSideLeft, "SL"},
	{SpeakerSideRight, "SR"},
}

// DefaultChannelMask returns the conventional WAVE speaker mask/order for
// common channel counts.
func DefaultChannelMask(channels int) uint32 {
	switch channels {
	case 1:
		return SpeakerFrontCenter
	case 2:
		return SpeakerFrontLeft | SpeakerFrontRight
	case 6:
		return SpeakerFrontLeft | SpeakerFrontRight | SpeakerFrontCenter | SpeakerLFE | SpeakerBackLeft | SpeakerBackRight
	case 8:
		return SpeakerFrontLeft | SpeakerFrontRight | SpeakerFrontCenter | SpeakerLFE |
			SpeakerBackLeft | SpeakerBackRight | SpeakerSideLeft | SpeakerSideRight
	default:
		return 0
	}
}

// ChannelNames returns names in WAV interleave order.
func ChannelNames(f Format) []string {
	mask := f.ChannelMask
	if mask == 0 {
		mask = DefaultChannelMask(f.Channels)
	}
	names := make([]string, 0, f.Channels)
	for _, speaker := range speakerOrder {
		if mask&speaker.bit != 0 && len(names) < f.Channels {
			names = append(names, speaker.name)
		}
	}
	for len(names) < f.Channels {
		names = append(names, fmt.Sprintf("CH%d", len(names)+1))
	}
	return names
}

// ChannelIndex resolves a speaker label or a zero-based channel index.
func ChannelIndex(f Format, spec string) (int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, fmt.Errorf("empty channel")
	}
	names := ChannelNames(f)
	upper := strings.ToUpper(spec)
	aliases := map[string]string{
		"CENTER": "FC", "DIALOGUE": "FC", "DIALOG": "FC", "C": "FC",
		"LOW": "LFE", "SUB": "LFE", "SW": "LFE",
		"LEFT": "FL", "RIGHT": "FR",
	}
	if alias, ok := aliases[upper]; ok {
		upper = alias
	}
	for i, name := range names {
		if strings.ToUpper(name) == upper {
			return i, nil
		}
	}
	var index int
	if _, err := fmt.Sscanf(spec, "%d", &index); err == nil && index >= 0 && index < len(names) {
		return index, nil
	}
	return 0, fmt.Errorf("unknown channel %q (available: %s)", spec, strings.Join(names, ", "))
}
