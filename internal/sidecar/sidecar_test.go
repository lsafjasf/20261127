package sidecar

import (
	"testing"

	"subdeliver/internal/manifest"
	"subdeliver/internal/subtitle"
)

func sampleSet() manifest.CueSet {
	return manifest.CueSet{
		Language: "en-US",
		Label:    "English",
		Cues: []subtitle.Cue{
			{Index: 1, Start: 0, End: 2_500_000_000, Text: "Hello world", Language: "en-US"},
			{Index: 2, Start: 2_500_000_000, End: 5_000_000_000,
				Text: "Line one\nLine two", Language: "en-US"},
		},
	}
}

func TestSRTRoundTrip(t *testing.T) {
	set := sampleSet()
	cues, err := ParseSRT(RenderSRT(set), set.Language)
	if err != nil {
		t.Fatal(err)
	}
	assertCues(t, set.Cues, cues)
}

func TestVTTRoundTrip(t *testing.T) {
	set := sampleSet()
	cues, err := ParseVTT(RenderVTT(set), set.Language)
	if err != nil {
		t.Fatal(err)
	}
	assertCues(t, set.Cues, cues)
}

func assertCues(t *testing.T, want, got []subtitle.Cue) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cue count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Start != want[i].Start || got[i].End != want[i].End {
			t.Errorf("cue %d timing = [%d,%d], want [%d,%d]",
				i, got[i].Start, got[i].End, want[i].Start, want[i].End)
		}
		if got[i].Text != want[i].Text {
			t.Errorf("cue %d text = %q, want %q", i, got[i].Text, want[i].Text)
		}
	}
}

func TestParseSRTMissingTiming(t *testing.T) {
	if _, err := ParseSRT([]byte("1\nnot a timing line\n"), "en"); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := ParseVTT([]byte("NOT VTT\n"), "en"); err == nil {
		t.Fatal("expected VTT header error")
	}
}
