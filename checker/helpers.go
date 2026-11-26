package checker

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"mediaqacontract/contract"
	"mediaqacontract/media"
)

func (r ItemResult) pass(expected, actual, msg string) ItemResult {
	r.Status, r.Expected, r.Actual, r.Message = StatusPass, expected, actual, msg
	return r
}

func (r ItemResult) fail(expected, actual, msg string) ItemResult {
	r.Status, r.Expected, r.Actual, r.Message = StatusFail, expected, actual, msg
	return r
}

func (r ItemResult) unknown(expected, actual, msg string) ItemResult {
	r.Status, r.Expected, r.Actual, r.Message = StatusUnknown, expected, actual, msg
	return r
}

func containsFold(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func regexpMatch(pattern, name string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(name), nil
}

func abs(x float64) float64 { return math.Abs(x) }

func minOne(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func describeAllowedRates(rule *contract.FrameRateRule) string {
	parts := make([]string, 0, len(rule.Allowed))
	for _, f := range rule.Allowed {
		v, err := f.Value()
		if err != nil {
			parts = append(parts, fmt.Sprintf("%d/%d", f.Num, f.Den))
			continue
		}
		parts = append(parts, strconv.FormatFloat(v, 'f', 3, 64))
	}
	return fmt.Sprintf("frame rate in %v (tolerance %.3f fps)", parts, rule.Tolerance)
}

func describeBitrate(rule *contract.BitrateRule) string {
	switch {
	case rule.MinBPS != 0 && rule.MaxBPS != 0:
		return fmt.Sprintf("bitrate between %d and %d bps", rule.MinBPS, rule.MaxBPS)
	case rule.MinBPS != 0:
		return fmt.Sprintf("bitrate >= %d bps", rule.MinBPS)
	default:
		return fmt.Sprintf("bitrate <= %d bps", rule.MaxBPS)
	}
}

func checkBitrate(id, category string, rule *contract.BitrateRule, bps int64) ItemResult {
	item := ItemResult{ID: id, Category: category}
	expected := describeBitrate(rule)
	if bps <= 0 {
		return item.unknown(expected, "", "probe did not report bitrate")
	}
	return evaluateBitrateItem(item, rule, []int64{bps}, 0)
}

func evaluateBitrateItem(item ItemResult, rule *contract.BitrateRule,
	values []int64, missingStreams int) ItemResult {
	expected := describeBitrate(rule)
	for _, bps := range values {
		if rule.MinBPS != 0 && bps < rule.MinBPS {
			return item.fail(expected, fmt.Sprintf("%d bps", bps),
				"bitrate below contract minimum")
		}
		if rule.MaxBPS != 0 && bps > rule.MaxBPS {
			return item.fail(expected, fmt.Sprintf("%d bps", bps),
				"bitrate above contract maximum")
		}
	}
	actual := fmt.Sprintf("%d measured stream(s)", len(values))
	if missingStreams > 0 {
		return item.unknown(expected, actual,
			fmt.Sprintf("%d stream(s) missing bitrate; measured streams pass", missingStreams))
	}
	return item.pass(expected, actual, "bitrate within contract range")
}

func checkAudioStreams(id, field string, allowed []string,
	streams []media.AudioStream, get func(media.AudioStream) string) ItemResult {
	item := ItemResult{ID: id, Category: "audio"}
	expected := fmt.Sprintf("%s in %v", field, allowed)
	for _, stream := range streams {
		value := get(stream)
		if value == "" {
			return item.unknown(expected, "",
				fmt.Sprintf("probe did not report audio %s", field))
		}
		if !containsFold(allowed, value) {
			return item.fail(expected, value,
				fmt.Sprintf("audio %s not allowed by contract", field))
		}
	}
	return item.pass(expected, "all streams match",
		fmt.Sprintf("all audio %s values allowed by contract", field))
}

func checkSubtitleField(id, field string, allowed []string,
	streams []media.SubtitleStream, get func(media.SubtitleStream) string) ItemResult {
	item := ItemResult{ID: id, Category: "subtitles"}
	expected := fmt.Sprintf("%s in %v", field, allowed)
	for _, stream := range streams {
		value := get(stream)
		if value == "" {
			return item.unknown(expected, "",
				fmt.Sprintf("probe did not report subtitle %s", field))
		}
		if !containsFold(allowed, value) {
			return item.fail(expected, value,
				fmt.Sprintf("subtitle %s not allowed by contract", field))
		}
	}
	return item.pass(expected, "all streams match",
		fmt.Sprintf("all subtitle %s values allowed by contract", field))
}

// canonicalLayout normalizes ffprobe-style channel layout names and falls
// back to channel-count aliases such as stereo/5.1 when no name exists.
func canonicalLayout(layout string, channels int) string {
	layout = strings.ToLower(strings.TrimSpace(layout))
	switch layout {
	case "mono", "stereo", "2.0", "5.1", "7.1":
		if layout == "2.0" {
			return "stereo"
		}
		return layout
	case "":
		// No layout name: use channel count, which the probe did provide.
	default:
		return layout
	}
	switch channels {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	default:
		if channels > 0 {
			return strconv.Itoa(channels) + " channels"
		}
		return ""
	}
}
