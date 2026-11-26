package manifest

import (
	"errors"
	"fmt"
	"strings"

	"subdeliver/internal/subtitle"
)

// Manifest is the single source of truth for every subtitle deliverable.
// Both the sidecar files and the embedded subtitle container are generated
// from, and verified against, this one manifest.
type Manifest struct {
	Version string  `json:"version"`
	MediaID string  `json:"media_id"`
	Tracks  []Track `json:"tracks"`
}

// Track is one language stream.
type Track struct {
	Language string `json:"language"`
	Label    string `json:"label,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Cues     []Cue  `json:"cues"`
}

// Cue is one timed text entry. Start/End use either SRT ("HH:MM:SS,mmm") or
// WebVTT ("HH:MM:SS.mmm") notation. Text may contain multiple lines.
type Cue struct {
	ID    string `json:"id,omitempty"`
	Start string `json:"start"`
	End   string `json:"end"`
	Text  string `json:"text"`
}

// Validate checks structural and semantic validity of the manifest.
func (m *Manifest) Validate() error {
	if strings.TrimSpace(m.MediaID) == "" {
		return errors.New("manifest: media_id is required")
	}
	if len(m.Tracks) == 0 {
		return errors.New("manifest: at least one track is required")
	}
	seenLang := make(map[string]bool, len(m.Tracks))
	defaults := 0
	for ti := range m.Tracks {
		t := &m.Tracks[ti]
		lang := strings.TrimSpace(t.Language)
		if lang == "" {
			return fmt.Errorf("manifest: track %d has empty language", ti)
		}
		if !subtitle.ValidLanguage(lang) {
			return fmt.Errorf("manifest: track %d has invalid BCP 47 language %q", ti, lang)
		}
		if seenLang[lang] {
			return fmt.Errorf("manifest: duplicate language %q", lang)
		}
		seenLang[lang] = true
		if t.Default {
			defaults++
		}
		if len(t.Cues) == 0 {
			return fmt.Errorf("manifest: track %q has no cues", lang)
		}
		for ci := range t.Cues {
			if err := t.Cues[ci].validate(ti, ci); err != nil {
				return err
			}
		}
	}
	if defaults > 1 {
		return fmt.Errorf("manifest: %d tracks marked default, at most one allowed", defaults)
	}
	return nil
}

func (c *Cue) validate(trackIdx, cueIdx int) error {
	start, err := subtitle.ParseTime(c.Start)
	if err != nil {
		return fmt.Errorf("manifest: track %d cue %d start: %w", trackIdx, cueIdx, err)
	}
	end, err := subtitle.ParseTime(c.End)
	if err != nil {
		return fmt.Errorf("manifest: track %d cue %d end: %w", trackIdx, cueIdx, err)
	}
	if end <= start {
		return fmt.Errorf("manifest: track %d cue %d: end %s must be after start %s",
			trackIdx, cueIdx, c.End, c.Start)
	}
	if strings.TrimSpace(c.Text) == "" {
		return fmt.Errorf("manifest: track %d cue %d: text is empty", trackIdx, cueIdx)
	}
	return nil
}
