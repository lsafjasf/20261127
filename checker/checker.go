// Package checker evaluates a machine-readable contract against a tool-neutral
// media probe and produces a per-item PASS / FAIL / UNKNOWN report.
package checker

import (
	"fmt"
	"path/filepath"
	"strings"

	"mediaqacontract/contract"
	"mediaqacontract/media"
)

// Verify runs every applicable contract item against the probe.
func Verify(c *contract.Contract, p *media.Probe) *Report {
	report := &Report{
		ContractID: c.ContractID,
		Platform:   c.Platform,
		File:       p.Path,
	}

	report.Items = append(report.Items, checkFilename(c, p))
	report.Items = append(report.Items, checkContainer(c, p))
	report.Items = append(report.Items, checkVideo(c, p)...)
	report.Items = append(report.Items, checkAudio(c, p)...)
	report.Items = append(report.Items, checkSubtitles(c, p)...)

	report.Overall = overall(report.Items)
	return report
}

func overall(items []ItemResult) Status {
	hasUnknown := false
	for _, item := range items {
		switch item.Status {
		case StatusFail:
			return StatusFail
		case StatusUnknown:
			hasUnknown = true
		}
	}
	if hasUnknown {
		return StatusUnknown
	}
	return StatusPass
}

func checkFilename(c *contract.Contract, p *media.Probe) ItemResult {
	rule := c.Filename
	item := ItemResult{ID: "filename", Category: "filename"}
	name := filepath.Base(p.Path)

	if len(rule.Extensions) > 0 {
		ext := strings.TrimPrefix(filepath.Ext(name), ".")
		if ext == "" {
			return item.fail(fmt.Sprintf("allowed extensions %v", rule.Extensions),
				"no extension", "filename has no extension")
		}
		if !containsFold(rule.Extensions, ext) {
			return item.fail(fmt.Sprintf("extension in %v", rule.Extensions),
				ext, "extension not allowed by contract")
		}
	}
	if rule.MaxLength > 0 && len([]rune(name)) > rule.MaxLength {
		return item.fail(fmt.Sprintf("length <= %d", rule.MaxLength),
			fmt.Sprintf("%d", len([]rune(name))), "filename exceeds max length")
	}
	if rule.Regex != "" {
		matched, err := regexpMatch(rule.Regex, name)
		if err != nil {
			return item.unknown("filename.regex", name,
				fmt.Sprintf("contract regex could not be evaluated: %v", err))
		}
		if !matched {
			return item.fail(fmt.Sprintf("match %s", rule.Regex), name,
				"filename does not match required pattern")
		}
	}
	return item.pass("filename rules", name, "filename satisfies naming rules")
}

func checkContainer(c *contract.Contract, p *media.Probe) ItemResult {
	item := ItemResult{ID: "container", Category: "container"}
	if len(c.Containers) == 0 {
		return item.pass("no container constraint", "", "container not constrained")
	}
	if p.Container == "" && len(p.ContainerAliases) == 0 {
		return item.unknown(fmt.Sprintf("container in %v", c.Containers), "",
			"probe did not report container format")
	}
	actual := p.Container
	for _, allowed := range c.Containers {
		if containsFold(p.ContainerAliases, allowed) {
			return item.pass(fmt.Sprintf("container in %v", c.Containers), allowed,
				"container allowed by contract")
		}
	}
	if !containsFold(c.Containers, actual) {
		return item.fail(fmt.Sprintf("container in %v", c.Containers), actual,
			"container not allowed by contract")
	}
	return item.pass(fmt.Sprintf("container in %v", c.Containers), actual,
		"container allowed by contract")
}

func checkVideo(c *contract.Contract, p *media.Probe) []ItemResult {
	rule := c.Video
	var items []ItemResult

	presence := ItemResult{ID: "video.presence", Category: "video"}
	if len(p.Video) == 0 {
		if rule.Required {
			items = append(items, presence.fail("video stream present", "0 streams",
				"required video stream is missing"))
		} else {
			items = append(items, presence.pass("video optional", "0 streams",
				"no video stream and video is optional"))
		}
		return items
	}
	items = append(items, presence.pass("video stream present",
		fmt.Sprintf("%d stream(s)", len(p.Video)), "video stream found"))

	stream := p.Video[0]
	if len(rule.Codecs) > 0 {
		item := ItemResult{ID: "video.codec", Category: "video"}
		if stream.Codec == "" {
			items = append(items, item.unknown(fmt.Sprintf("codec in %v", rule.Codecs), "",
				"probe did not report video codec"))
		} else if !containsFold(rule.Codecs, stream.Codec) {
			items = append(items, item.fail(fmt.Sprintf("codec in %v", rule.Codecs),
				stream.Codec, "video codec not allowed by contract"))
		} else {
			items = append(items, item.pass(fmt.Sprintf("codec in %v", rule.Codecs),
				stream.Codec, "video codec allowed by contract"))
		}
	}

	if rule.FrameRate != nil {
		items = append(items, checkFrameRate(rule, stream))
	}
	if rule.Bitrate != nil {
		items = append(items, checkBitrate("video.bitrate", "video", rule.Bitrate, stream.BPS))
	}
	return items
}

func checkFrameRate(rule contract.VideoRule, stream media.VideoStream) ItemResult {
	item := ItemResult{ID: "video.frameRate", Category: "video"}
	expected := describeAllowedRates(rule.FrameRate)

	fps, err := stream.FrameRate.FPS()
	if err != nil {
		return item.unknown(expected, fmt.Sprintf("%d/%d", stream.FrameRate.Num, stream.FrameRate.Den),
			fmt.Sprintf("probe reported invalid frame rate: %v", err))
	}
	if stream.FrameRate.Num == 0 {
		return item.unknown(expected, "", "probe did not report video frame rate")
	}
	for _, allowed := range rule.FrameRate.Allowed {
		target, err := allowed.Value()
		if err != nil {
			return item.unknown(expected, "", fmt.Sprintf("invalid contract rate: %v", err))
		}
		if abs(fps-target) <= rule.FrameRate.Tolerance {
			return item.pass(expected, fmt.Sprintf("%.3f fps", fps),
				"frame rate within tolerance of an allowed rate")
		}
	}
	return item.fail(expected, fmt.Sprintf("%.3f fps", fps),
		"frame rate does not match any allowed rate")
}

func checkAudio(c *contract.Contract, p *media.Probe) []ItemResult {
	rule := c.Audio
	var items []ItemResult

	count := ItemResult{ID: "audio.streams", Category: "audio"}
	n := len(p.Audio)
	switch {
	case n == 0 && rule.Required:
		items = append(items, count.fail("audio stream present", "0 streams",
			"required audio stream is missing"))
		return items
	case n == 0:
		items = append(items, count.pass("audio optional", "0 streams",
			"no audio stream and audio is optional"))
		return items
	case rule.MaxStreams != 0 && n > rule.MaxStreams:
		items = append(items, count.fail(
			fmt.Sprintf("streams between %d and %d", minOne(rule.MinStreams), rule.MaxStreams),
			fmt.Sprintf("%d streams", n), "too many audio streams"))
	case rule.MinStreams != 0 && n < rule.MinStreams:
		items = append(items, count.fail(
			fmt.Sprintf("min %d audio stream(s)", rule.MinStreams),
			fmt.Sprintf("%d streams", n), "not enough audio streams"))
	default:
		items = append(items, count.pass("audio stream count rule",
			fmt.Sprintf("%d streams", n), "audio stream count within range"))
	}

	if len(rule.Codecs) > 0 {
		items = append(items, checkAudioStreams("audio.codec", "codec", rule.Codecs,
			p.Audio, func(s media.AudioStream) string { return s.Codec }))
	}
	if len(rule.ChannelLayouts) > 0 {
		items = append(items, checkAudioLayouts(rule, p.Audio))
	}
	if rule.Bitrate != nil {
		var known, missing []int64
		for _, stream := range p.Audio {
			if stream.BPS <= 0 {
				missing = append(missing, stream.BPS)
				continue
			}
			known = append(known, stream.BPS)
		}
		item := ItemResult{ID: "audio.bitrate", Category: "audio"}
		if len(known) == 0 {
			items = append(items, item.unknown(describeBitrate(rule.Bitrate), "",
				"probe did not report audio bitrate for any stream"))
		} else {
			items = append(items, evaluateBitrateItem(item, rule.Bitrate, known, len(missing)))
		}
	}
	return items
}

func checkAudioLayouts(rule contract.AudioRule, streams []media.AudioStream) ItemResult {
	item := ItemResult{ID: "audio.channelLayout", Category: "audio"}
	expected := fmt.Sprintf("layout in %v", rule.ChannelLayouts)
	for _, stream := range streams {
		canonical := canonicalLayout(stream.ChannelLayout, stream.Channels)
		if canonical == "" {
			return item.unknown(expected,
				fmt.Sprintf("layout=%q channels=%d", stream.ChannelLayout, stream.Channels),
				"probe did not report a recognizable channel layout")
		}
		if !containsFold(rule.ChannelLayouts, canonical) {
			return item.fail(expected, canonical,
				"audio channel layout not allowed by contract")
		}
	}
	return item.pass(expected, "all streams match", "all audio layouts allowed by contract")
}

func checkSubtitles(c *contract.Contract, p *media.Probe) []ItemResult {
	rule := c.Subtitles
	var items []ItemResult

	presence := ItemResult{ID: "subtitles.presence", Category: "subtitles"}
	if len(p.Subtitles) == 0 {
		if rule.Required {
			items = append(items, presence.fail("subtitle stream present", "0 streams",
				"required subtitle stream is missing"))
			return items
		}
		items = append(items, presence.pass("subtitles optional", "0 streams",
			"no subtitle stream and subtitles are optional"))
		return items
	}
	items = append(items, presence.pass("subtitle stream present",
		fmt.Sprintf("%d stream(s)", len(p.Subtitles)), "subtitle stream found"))

	if len(rule.Codecs) > 0 {
		items = append(items, checkSubtitleField("subtitles.codec", "codec", rule.Codecs,
			p.Subtitles, func(s media.SubtitleStream) string { return s.Codec }))
	}
	if len(rule.Languages) > 0 {
		items = append(items, checkSubtitleField("subtitles.language", "language", rule.Languages,
			p.Subtitles, func(s media.SubtitleStream) string { return s.Language }))
	}
	return items
}
