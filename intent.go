package iccproof

import (
	"errors"
	"fmt"
	"strings"
)

// Intent identifies one explicit ICC rendering intent.
//
// The numeric values are intentionally identical to ICC/Little CMS intent
// codes. The empty string is not a valid intent: callers must choose one
// explicitly or use AllProofIntents for the three-way comparison.
type Intent string

const (
	IntentPerceptual           Intent = "perceptual"
	IntentRelativeColorimetric Intent = "relative_colorimetric"
	IntentSaturation           Intent = "saturation"
	IntentAbsoluteColorimetric Intent = "absolute_colorimetric"
)

// AllProofIntents are the three soft-proofing intents required by the task.
func AllProofIntents() []Intent {
	return []Intent{
		IntentPerceptual,
		IntentRelativeColorimetric,
		IntentAbsoluteColorimetric,
	}
}

var ErrInvalidIntent = errors.New("invalid ICC rendering intent")

// ParseIntent accepts stable CLI/JSON names and common aliases. It deliberately
// rejects an empty value instead of silently choosing perceptual.
func ParseIntent(raw string) (Intent, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.NewReplacer("-", "_", " ", "_").Replace(key)
	key = strings.Trim(key, "_")
	switch key {
	case "0", "p", "perceptual", "perc", "感知", "感知意图":
		return IntentPerceptual, nil
	case "1", "r", "rel", "relative", "relative_colorimetric", "relativecolorimetric", "相对色度", "相对色度意图":
		return IntentRelativeColorimetric, nil
	case "2", "s", "sat", "saturation", "饱和", "饱和度":
		return IntentSaturation, nil
	case "3", "a", "abs", "absolute", "absolute_colorimetric", "absolutecolori", "absolutecolorimetric", "绝对色度", "绝对色度意图":
		return IntentAbsoluteColorimetric, nil
	case "":
		return "", fmt.Errorf("%w: empty intent; choose perceptual, relative-colorimetric or absolute-colorimetric", ErrInvalidIntent)
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidIntent, raw)
	}
}

// ParseIntents parses a comma-separated list. "all" expands to the three
// proofing intents used in the comparison report.
func ParseIntents(raw string) ([]Intent, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "all") {
		return AllProofIntents(), nil
	}
	parts := strings.Split(raw, ",")
	out := make([]Intent, 0, len(parts))
	seen := map[Intent]bool{}
	for _, part := range parts {
		intent, err := ParseIntent(part)
		if err != nil {
			return nil, err
		}
		if !seen[intent] {
			out = append(out, intent)
			seen[intent] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no intents", ErrInvalidIntent)
	}
	return out, nil
}

// ICCValue returns the ICC intent number.
func (i Intent) ICCValue() uint32 {
	switch i {
	case IntentPerceptual:
		return 0
	case IntentRelativeColorimetric:
		return 1
	case IntentSaturation:
		return 2
	case IntentAbsoluteColorimetric:
		return 3
	default:
		return 0xFFFFFFFF
	}
}

func (i Intent) Valid() bool { return i.ICCValue() <= 3 }

// FileNameSlug returns a filesystem/CSV-safe spelling.
func (i Intent) FileNameSlug() string { return string(i) }

func (i Intent) String() string { return string(i) }
