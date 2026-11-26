// Package verify compares the generated deliverables against the single
// source manifest, cue by cue, covering language, timing and text.
package verify

import (
	"fmt"
	"strings"

	"subdeliver/internal/manifest"
	"subdeliver/internal/subtitle"
)

// Mismatch describes one concrete difference between the source manifest and
// a generated deliverable.
type Mismatch struct {
	Deliverable string // "sidecar:srt", "sidecar:vtt" or "embedded"
	Language    string
	CueIndex    int
	Field       string // "language", "start", "end", "text", "cue_count"
	Expected    string
	Actual      string
}

func (m Mismatch) String() string {
	return fmt.Sprintf("[%s] lang=%s cue=%d field=%s: expected %q, got %q",
		m.Deliverable, m.Language, m.CueIndex, m.Field, m.Expected, m.Actual)
}

// Report aggregates every detected inconsistency. A non-empty report blocks
// the release (the CLI exits non-zero).
type Report struct {
	Mismatches []Mismatch
}

// Add records a mismatch.
func (r *Report) Add(deliverable, lang string, cue int, field string, want, got any) {
	r.Mismatches = append(r.Mismatches, Mismatch{
		Deliverable: deliverable,
		Language:    lang,
		CueIndex:    cue,
		Field:       field,
		Expected:    fmt.Sprint(want),
		Actual:      fmt.Sprint(got),
	})
}

// OK reports whether every deliverable matches the source manifest.
func (r *Report) OK() bool { return len(r.Mismatches) == 0 }

// Text renders all mismatches for human-readable release logs.
func (r *Report) Text() string {
	lines := make([]string, 0, len(r.Mismatches))
	for _, m := range r.Mismatches {
		lines = append(lines, m.String())
	}
	return strings.Join(lines, "\n")
}

// Sidecar compares one reparsed sidecar track with its source cues.
func (r *Report) Sidecar(deliverable string, want manifest.CueSet, got []subtitle.Cue) {
	if len(got) != len(want.Cues) {
		r.Add(deliverable, want.Language, 0, "cue_count", len(want.Cues), len(got))
		return
	}
	for i, src := range want.Cues {
		actual := got[i]
		compareCue(r, deliverable, want.Language, i+1, src, actual)
	}
}

// Embedded compares the reparsed embedded container with every source track.
func (r *Report) Embedded(want []manifest.CueSet, got []manifest.CueSet) {
	if len(got) != len(want) {
		r.Add("embedded", "-", 0, "track_count", len(want), len(got))
	}
	gotByLang := make(map[string]manifest.CueSet, len(got))
	for _, set := range got {
		gotByLang[set.Language] = set
	}
	for _, src := range want {
		actual, ok := gotByLang[src.Language]
		if !ok {
			r.Add("embedded", src.Language, 0, "language", "present", "missing")
			continue
		}
		if actual.Default != src.Default {
			r.Add("embedded", src.Language, 0, "default", src.Default, actual.Default)
		}
		if len(actual.Cues) != len(src.Cues) {
			r.Add("embedded", src.Language, 0, "cue_count",
				len(src.Cues), len(actual.Cues))
			continue
		}
		for i, cue := range src.Cues {
			compareCue(r, "embedded", src.Language, i+1, cue, actual.Cues[i])
		}
	}
}

func compareCue(r *Report, deliverable, lang string, index int, want, got subtitle.Cue) {
	if got.Language != lang {
		r.Add(deliverable, lang, index, "language", lang, got.Language)
	}
	if got.Start != want.Start {
		r.Add(deliverable, lang, index, "start",
			subtitle.FormatESUB(want.Start), subtitle.FormatESUB(got.Start))
	}
	if got.End != want.End {
		r.Add(deliverable, lang, index, "end",
			subtitle.FormatESUB(want.End), subtitle.FormatESUB(got.End))
	}
	wantText := subtitle.NormalizeText(want.Text)
	gotText := subtitle.NormalizeText(got.Text)
	if gotText != wantText {
		r.Add(deliverable, lang, index, "text", wantText, gotText)
	}
}
