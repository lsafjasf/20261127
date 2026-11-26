package seek

import (
	"os"
	"testing"
	"time"

	"seekdiag/internal/mp4"
	"seekdiag/internal/testmedia"
)

func TestSeekTwentyMinutesWithEditList(t *testing.T) {
	path := t.TempDir() + "/audio.mp4"
	if err := os.WriteFile(path, testmedia.BuildAudioMP4(), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := mp4.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := At(f, 20*time.Minute+5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tracks) != 1 {
		t.Fatalf("tracks = %d", len(plan.Tracks))
	}
	landing := plan.Tracks[0].Boundary.Landing
	if landing == nil {
		t.Fatal("missing landing sample")
	}
	if landing.SampleIndex != 56_252 {
		t.Fatalf("landing sample = %d, want 56252", landing.SampleIndex)
	}
	if landing.SamplePTSMS != 1_200_000 || landing.TrimBeforeMS != 5 {
		t.Fatalf("landing pts=%.3f trim=%.3f", landing.SamplePTSMS, landing.TrimBeforeMS)
	}
	if landing.PCMStart != 57_600_000 {
		t.Fatalf("landing PCM = %d, want 57600000", landing.PCMStart)
	}
	if landing.FileOffset != 908_016 || landing.OffsetInChunk != 4_016 {
		t.Fatalf("landing file offset=%d inChunk=%d", landing.FileOffset, landing.OffsetInChunk)
	}
}
