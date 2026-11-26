package sidecar

import (
	"fmt"
	"strings"

	"subdeliver/internal/manifest"
	"subdeliver/internal/subtitle"
)

// RenderVTT renders one track as a standard WebVTT sidecar file.
func RenderVTT(set manifest.CueSet) []byte {
	var b strings.Builder
	b.WriteString("WEBVTT\n")
	if set.Label != "" {
		b.WriteString("- ")
		b.WriteString(set.Label)
	}
	b.WriteString("\n\n")
	for _, cue := range set.Cues {
		if cue.ID != "" {
			b.WriteString(cue.ID)
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s --> %s\n",
			subtitle.FormatVTT(cue.Start), subtitle.FormatVTT(cue.End))
		b.WriteString(cue.Text)
		b.WriteString("\n\n")
	}
	return []byte(b.String())
}

// ParseVTT independently reads back a WebVTT file for verification.
func ParseVTT(data []byte, language string) ([]subtitle.Cue, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	blocks := strings.Split(strings.TrimSpace(text), "\n\n")
	if len(blocks) == 0 || !strings.HasPrefix(blocks[0], "WEBVTT") {
		return nil, fmt.Errorf("vtt: missing WEBVTT header")
	}
	cues := make([]subtitle.Cue, 0, len(blocks))
	for bi := 1; bi < len(blocks); bi++ {
		block := strings.TrimSpace(blocks[bi])
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		pos := 0
		id := ""
		if !strings.Contains(lines[0], "-->") {
			id = strings.TrimSpace(lines[0])
			pos = 1
		}
		if pos >= len(lines) {
			return nil, fmt.Errorf("vtt: block %d missing timing line", bi)
		}
		start, end, err := parseVTTTiming(lines[pos])
		if err != nil {
			return nil, fmt.Errorf("vtt: block %d: %w", bi, err)
		}
		pos++
		cues = append(cues, subtitle.Cue{
			Index:    len(cues) + 1,
			ID:       id,
			Start:    start,
			End:      end,
			Text:     subtitle.NormalizeText(strings.Join(lines[pos:], "\n")),
			Language: language,
		})
	}
	return cues, nil
}

func parseVTTTiming(line string) (int64, int64, error) {
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
