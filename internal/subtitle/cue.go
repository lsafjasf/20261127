package subtitle

import (
	"regexp"
	"strings"
)

// Cue is the canonical, format-independent subtitle entry shared by every
// deliverable. Time is stored in nanoseconds for lossless comparison.
type Cue struct {
	Index    int
	ID       string
	Start    int64
	End      int64
	Text     string
	Language string
}

// languageTag matches a BCP 47-ish tag: primary 2*3ALPHA with optional
// region/script/variant subtags separated by hyphens (e.g. zh, en-US,
// zh-Hant, pt-BR).
var languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// ValidLanguage reports whether lang is a syntactically plausible BCP 47 tag.
func ValidLanguage(lang string) bool {
	return languageTag.MatchString(strings.TrimSpace(lang))
}

// NormalizeText canonicalizes cue text so that format-specific whitespace
// differences (CRLF, trailing spaces, blank padding lines) cannot produce
// false mismatches between sidecar and embedded deliverables.
func NormalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed = append(trimmed, line)
	}
	return strings.Trim(strings.Join(trimmed, "\n"), "\n")
}
