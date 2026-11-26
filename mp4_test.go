package laddercheck

import (
	"bytes"
	"testing"
)

func parseSpecBytes(t *testing.T, spec FixtureSpec) RenditionFacts {
	t.Helper()
	data, err := BuildFixtureMP4(spec)
	if err != nil {
		t.Fatalf("BuildFixtureMP4: %v", err)
	}
	f, err := ParseMP4(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ParseMP4: %v", err)
	}
	return *f
}

func TestFixtureParsesTimelineAndEditList(t *testing.T) {
	f := parseSpecBytes(t, FixtureSpec{
		ID: "x", MovieTimescale: 30000, MediaTimescale: 30000,
		Frames: 150, FrameDurationTicks: 1000, KeyframeSamples: []int{0, 60, 120},
		Edits: []FixtureEdit{
			{Empty: true, SegmentDurationTicks: 1000},
			{SegmentDurationTicks: 150000, MediaTimeTicks: 0},
		},
	})
	if f.SampleCount != 150 || f.FPS.String() != "30" {
		t.Fatalf("count=%d fps=%s", f.SampleCount, f.FPS)
	}
	if len(f.Keyframes) != 3 {
		t.Fatalf("keyframes=%d want 3", len(f.Keyframes))
	}
	if f.FirstDisplayPTS.Cmp(NewRat(1, 30)) != 0 {
		t.Fatalf("first display=%s want 1/30", f.FirstDisplayPTS)
	}
	if got := f.Keyframes[1].DisplayPTS; got.Cmp(NewRat(61, 30)) != 0 {
		t.Fatalf("key60 display=%s want 61/30 (2+1/30)", got)
	}
	for _, k := range f.Keyframes {
		if k.IDR == nil || !*k.IDR {
			t.Fatalf("sample %d expected IDR, got %v", k.Sample, k.IDR)
		}
	}
	if !f.GOPUniform || len(f.GOPDisplayDurations) != 2 ||
		f.GOPDisplayDurations[0].Cmp(NewRat(2, 1)) != 0 {
		t.Fatalf("gop uniform=%v durations=%v", f.GOPUniform, f.GOPDisplayDurations)
	}
}

func TestFixtureCTOFirstPTSOffset(t *testing.T) {
	f := parseSpecBytes(t, FixtureSpec{
		MediaTimescale: 15360, Frames: 150, FrameDurationTicks: 512,
		CTOConstant: 1536, KeyframeSamples: []int{0, 60, 120},
	})
	if f.FirstPTS.Cmp(NewRat(1, 10)) != 0 {
		t.Fatalf("first pts=%s want 1/10", f.FirstPTS)
	}
	if f.EarliestPTS.Cmp(NewRat(1, 10)) != 0 {
		t.Fatalf("earliest pts=%s", f.EarliestPTS)
	}
	if f.Keyframes[1].PTS.Cmp(NewRat(21, 10)) != 0 {
		t.Fatalf("key60 pts=%s want 21/10", f.Keyframes[1].PTS)
	}
}

func TestFixtureHEVCNonIDRDetection(t *testing.T) {
	f := parseSpecBytes(t, FixtureSpec{
		Family: "hevc", MediaTimescale: 30000, Frames: 150, FrameDurationTicks: 1000,
		KeyframeSamples: []int{0, 60, 120}, NonIDR: map[int]bool{60: true, 120: true},
	})
	if f.CodecFamily != "hevc" {
		t.Fatalf("family=%s", f.CodecFamily)
	}
	if f.Keyframes[0].IDR == nil || !*f.Keyframes[0].IDR {
		t.Fatalf("sample0 should be IDR")
	}
	if f.Keyframes[1].IDR == nil || *f.Keyframes[1].IDR {
		t.Fatalf("sample60 should be known non-IDR (CRA), got %v", f.Keyframes[1].IDR)
	}
}

func TestParseRejectsNonMP4(t *testing.T) {
	if _, err := ParseMP4(bytes.NewReader([]byte("not an mp4 at all")), 18); err == nil {
		t.Fatal("expected error for non-MP4 input")
	}
}
