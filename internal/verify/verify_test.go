package verify

import (
	"strings"
	"testing"

	"subdeliver/internal/manifest"
	"subdeliver/internal/subtitle"
)

func sourceSet() manifest.CueSet {
	return manifest.CueSet{
		Language: "zh",
		Cues: []subtitle.Cue{
			{Index: 1, Start: 0, End: 1_000_000_000, Text: "你好", Language: "zh"},
			{Index: 2, Start: 1_000_000_000, End: 2_000_000_000, Text: "再見", Language: "zh"},
		},
	}
}

func TestSidecarMatch(t *testing.T) {
	src := sourceSet()
	r := &Report{}
	r.Sidecar("sidecar:srt", src, src.Cues)
	if !r.OK() {
		t.Fatalf("unexpected mismatches: %s", r.Text())
	}
}

func TestSidecarDetectsEveryField(t *testing.T) {
	src := sourceSet()
	mutated := []subtitle.Cue{
		{Index: 1, Start: 500_000_000, End: 1_000_000_000, Text: "你好", Language: "zh"},
		{Index: 2, Start: 1_000_000_000, End: 2_000_000_000, Text: "不同文本", Language: "en"},
	}
	r := &Report{}
	r.Sidecar("sidecar:vtt", src, mutated)
	text := r.Text()
	for _, field := range []string{"start", "text", "language"} {
		if !strings.Contains(text, "field="+field) {
			t.Errorf("expected mismatch on field %s in:\n%s", field, text)
		}
	}
}

func TestCueCountMismatch(t *testing.T) {
	src := sourceSet()
	r := &Report{}
	r.Sidecar("sidecar:srt", src, src.Cues[:1])
	if r.OK() || !strings.Contains(r.Text(), "field=cue_count") {
		t.Fatalf("expected cue_count mismatch, got:\n%s", r.Text())
	}
}

func TestEmbeddedTrackAndDefault(t *testing.T) {
	src := []manifest.CueSet{sourceSet()}
	got := []manifest.CueSet{{
		Language: "zh",
		Default:  true,
		Cues:     src[0].Cues,
	}}
	r := &Report{}
	r.Embedded(src, got)
	if !strings.Contains(r.Text(), "field=default") {
		t.Fatalf("expected default flag mismatch, got:\n%s", r.Text())
	}

	r2 := &Report{}
	r2.Embedded(src, nil)
	if !strings.Contains(r2.Text(), "field=track_count") {
		t.Fatalf("expected track_count mismatch, got:\n%s", r2.Text())
	}
}
