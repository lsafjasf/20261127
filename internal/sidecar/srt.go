package sidecar

import (
	"fmt"
	"strconv"
	"strings"

	"subdeliver/internal/manifest"
	"subdeliver/internal/subtitle"
)

// RenderSRT renders one track as a standard SRT sidecar file.
func RenderSRT(set manifest.CueSet) []byte {
	var b strings.Builder
	for i, cue := range set.Cues {
		if i > 0 {
			b.WriteString("\r\n")
		}
		fmt.Fprintf(&b, "%d\r\n", i+1)
		fmt.Fprintf(&b, "%s --> %s\r\n",
			subtitle.FormatSRT(cue.Start), subtitle.FormatSRT(cue.End))
		b.WriteString(strings.ReplaceAll(cue.Text, "\n", "\r\n"))
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

// ParseSRT is an independent reader used by the verification step. It does not
// share rendering code, so a round-trip mismatch is genuinely detectable.
func ParseSRT(data []byte, language string) ([]subtitle.Cue, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	blocks := strings.Split(strings.TrimSpace(text), "\n\n")
	cues := make([]subtitle.Cue, 0, len(blocks))
	for bi, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		pos := 0
		index := bi + 1
		if len(lines) > 0 && isDigits(strings.TrimSpace(lines[0])) {
			index, _ = strconv.Atoi(strings.TrimSpace(lines[0]))
			pos = 1
		}
		if pos >= len(lines) {
			return nil, fmt.Errorf("srt: block %d missing timing line", bi+1)
		}
		start, end, err := parseSRTTiming(lines[pos])
		if err != nil {
			return nil, fmt.Errorf("srt: block %d: %w", bi+1, err)
		}
		pos++
		body := strings.Join(lines[pos:], "\n")
		cues = append(cues, subtitle.Cue{
			Index:    index,
			Start:    start,
			End:      end,
			Text:     subtitle.NormalizeText(body),
			Language: language,
		})
	}
	return cues, nil
}

func parseSRTTiming(line string) (int64, int64, error) {
	parts := strings.Split(line, "-->")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid timing line %q", line)
	}
	start, err := subtitle.ParseTime(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	tail := strings.TrimSpace(parts[1])
	if idx := strings.Index(tail, " "); idx >= 0 {
		tail = tail[:idx]
	}
	end, err := subtitle.ParseTime(tail)
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
