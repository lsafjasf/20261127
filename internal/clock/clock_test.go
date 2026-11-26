package clock

import (
	"errors"
	"os"
	"testing"
	"time"

	"seekdiag/internal/mp4"
	"seekdiag/internal/seek"
	"seekdiag/internal/testmedia"
)

func TestClockAnchorsOnce(t *testing.T) {
	now := time.Unix(0, 0)
	c := New(func() time.Time { return now })
	if err := c.Anchor(1_200_000, now, 1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Second)
	got, ok := c.MediaMS()
	if !ok || got != 1_205_000 {
		t.Fatalf("MediaMS = %.3f, %v", got, ok)
	}
	if err := c.Anchor(1_205_000, now, 1); !errors.Is(err, ErrAlreadyAnchored) {
		t.Fatalf("second anchor error = %v, want ErrAlreadyAnchored", err)
	}
}

func TestAudioTimelineUsesAudiblePCMAnchor(t *testing.T) {
	a := AudioTimeline{AnchorMovieMS: 1_200_005, AnchorPCM: 57_600_240, SampleRate: 48_000}
	if got := a.MovieMS(57_600_240 + 48_000); got != 1_201_005 {
		t.Fatalf("movie time = %.3f", got)
	}
	if got := a.PCMAt(1_201_005); got != 57_648_240 {
		t.Fatalf("PCM position = %d", got)
	}
}

func TestAnchorFromSeekUsesEditListTarget(t *testing.T) {
	path := t.TempDir() + "/audio.mp4"
	if err := os.WriteFile(path, testmedia.BuildAudioMP4(), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := mp4.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := seek.At(f, 20*time.Minute+5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := AnchorFromSeek(plan)
	if err != nil {
		t.Fatal(err)
	}
	if anchor.MovieMS != 1_200_005 || anchor.Audio.AnchorPCM != 57_600_240 {
		t.Fatalf("anchor = %+v", anchor)
	}
}
