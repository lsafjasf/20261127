package embedded

import (
	"testing"

	"subdeliver/internal/manifest"
	"subdeliver/internal/subtitle"
)

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	sets := []manifest.CueSet{{
		Language: "zh-Hant",
		Label:    "繁體中文",
		Default:  true,
		Cues: []subtitle.Cue{
			{Index: 1, Start: 0, End: 1_500_000_000, Text: "第一行\n第二行", Language: "zh-Hant"},
			{Index: 2, Start: 2_000_000_000, End: 3_000_000_001, Text: "再見", Language: "zh-Hant"},
		},
	}}
	data, err := Marshal("movie42", sets)
	if err != nil {
		t.Fatal(err)
	}
	mediaID, got, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if mediaID != "movie42" {
		t.Fatalf("mediaID = %q", mediaID)
	}
	if len(got) != 1 || got[0].Language != "zh-Hant" || !got[0].Default {
		t.Fatalf("track mismatch: %+v", got)
	}
	if len(got[0].Cues) != 2 {
		t.Fatalf("cue count = %d", len(got[0].Cues))
	}
	c := got[0].Cues[1]
	if c.End != 3_000_000_001 || c.Text != "再見" {
		t.Fatalf("cue not preserved: %+v", c)
	}
}

func TestUnmarshalRejectsCorrupt(t *testing.T) {
	if _, _, err := Unmarshal([]byte("NOPE1\n")); err == nil {
		t.Fatal("expected bad magic error")
	}
	if _, _, err := Unmarshal(append([]byte(Magic), 0xff, 0xff)); err == nil {
		t.Fatal("expected truncation error")
	}
}
