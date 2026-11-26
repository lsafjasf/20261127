package main

import (
	"image"
	"image/color"
	"testing"
)

func loadTestData(t *testing.T) ([]Clip, []Source) {
	t.Helper()
	var clips []Clip
	var pool []Source
	if err := loadJSON("testdata/sequence.json", &clips); err != nil {
		t.Fatal(err)
	}
	if err := loadJSON("testdata/pool.json", &pool); err != nil {
		t.Fatal(err)
	}
	return clips, pool
}

func TestMapSeqToSrcConstantSpeed(t *testing.T) {
	c := Clip{ID: "x", SeqIn: 0, SeqOut: 50, SrcIn: 100, SrcOut: 150}
	got, err := c.MapSeqToSrc(25)
	if err != nil || got != 125 {
		t.Fatalf("got %d, %v; want 125", got, err)
	}
}

func TestMapSeqToSrcSpeedUp(t *testing.T) {
	c := Clip{ID: "x", SeqIn: 50, SeqOut: 75, SrcIn: 40, SrcOut: 90} // 2x
	got, err := c.MapSeqToSrc(60)
	if err != nil || got != 60 {
		t.Fatalf("got %d, %v; want 60", got, err)
	}
}

func TestMapSeqToSrcReverse(t *testing.T) {
	c := Clip{ID: "x", SeqIn: 75, SeqOut: 126, SrcIn: 500, SrcOut: 449} // -1x
	got, err := c.MapSeqToSrc(125)
	if err != nil || got != 450 {
		t.Fatalf("got %d, %v; want 450", got, err)
	}
}

func TestMapSeqToSrcOutOfRange(t *testing.T) {
	c := Clip{ID: "x", SeqIn: 0, SeqOut: 50, SrcIn: 100, SrcOut: 150}
	if _, err := c.MapSeqToSrc(50); err == nil {
		t.Fatal("expected error for frame at SeqOut (exclusive)")
	}
}

func TestMatchAll(t *testing.T) {
	clips, pool := loadTestData(t)
	reports := matchAll(clips, pool)
	if len(reports) != 5 {
		t.Fatalf("want 5 reports, got %d", len(reports))
	}
	byID := map[string]ClipReport{}
	for _, r := range reports {
		byID[r.Clip.ID] = r
	}

	if r := byID["c1"]; r.Status != "unique" || r.Candidates[0] != "/media/a/take_01.mov" {
		t.Errorf("c1: %+v", r)
	}
	// Same file name as c1's match, but 2x speed: fingerprints must pick /media/b.
	if r := byID["c2"]; r.Status != "unique" || r.Candidates[0] != "/media/b/take_01.mov" {
		t.Errorf("c2: %+v", r)
	}
	// Two sources carry identical endpoint fingerprints: cannot be unique.
	if r := byID["c3"]; r.Status != "ambiguous" || len(r.Candidates) != 2 {
		t.Errorf("c3: %+v", r)
	}
	if r := byID["c4"]; r.Status != "unmatched" {
		t.Errorf("c4: %+v", r)
	}
	// Name disagrees but fingerprints confirm: still a unique match.
	if r := byID["c5"]; r.Status != "unique" || r.NameAgrees {
		t.Errorf("c5: %+v", r)
	}
}

func TestMatchIgnoresName(t *testing.T) {
	// A source with the right name but wrong frames must not match.
	pool := []Source{{
		Path: "/media/x/take_01.mov", Name: "take_01.mov",
		Frames: []FrameHash{{Frame: 100, Hash: 1}, {Frame: 149, Hash: 2}},
	}}
	c := Clip{ID: "c", Name: "take_01.mov", SeqIn: 0, SeqOut: 50,
		SrcIn: 100, SrcOut: 150, FirstHash: 9, LastHash: 9}
	cands, err := matchCandidates(&c, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 0 {
		t.Fatalf("name-only match slipped through: %v", cands)
	}
}

func TestFingerprintStable(t *testing.T) {
	// The same horizontal gradient at two resolutions must hash
	// identically: a render round-trip changes size, not content.
	gradient := func(w, h int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := uint8(x * 255 / w)
				img.Set(x, y, color.RGBA{v, v, v, 255})
			}
		}
		return img
	}
	a := Fingerprint(gradient(320, 180))
	b := Fingerprint(gradient(1920, 1080))
	if Hamming(a, b) > 8 {
		t.Fatalf("gradient fingerprints diverged: %016x vs %016x (dist %d)",
			a, b, Hamming(a, b))
	}
}
