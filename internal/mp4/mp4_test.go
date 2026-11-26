package mp4

import (
	"os"
	"testing"

	"seekdiag/internal/testmedia"
)

func TestParseAudioFixture(t *testing.T) {
	path := t.TempDir() + "/audio.mp4"
	if err := os.WriteFile(path, testmedia.BuildAudioMP4(), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(f.Tracks))
	}
	tr := f.Tracks[0]
	if tr.ID != 1 || tr.Kind() != "audio" || tr.Codec != "mp4a" {
		t.Fatalf("unexpected track: id=%d kind=%s codec=%s", tr.ID, tr.Kind(), tr.Codec)
	}
	if tr.Timescale != 48_000 || tr.SampleRate != 48_000 || tr.ChannelCount != 2 {
		t.Fatalf("unexpected audio format: ts=%d rate=%d channels=%d", tr.Timescale, tr.SampleRate, tr.ChannelCount)
	}
	if len(tr.Samples) != 84_375 || len(tr.Edits) != 1 {
		t.Fatalf("samples=%d edits=%d", len(tr.Samples), len(tr.Edits))
	}
	if tr.Samples[0].FileOffset != 8_000 {
		t.Fatalf("sample 1 offset = %d, want 8000", tr.Samples[0].FileOffset)
	}
	if tr.Samples[1_000].FileOffset != 24_000 {
		t.Fatalf("sample 1001 offset = %d, want 24000", tr.Samples[1_000].FileOffset)
	}
	media, edit, ok := tr.MovieToMedia(1_200_000)
	if !ok || edit != 0 || media != 57_601_024 {
		t.Fatalf("movie 20m maps to media=%d edit=%d ok=%v", media, edit, ok)
	}
	movie, ok := tr.MediaToMovie(1_024)
	if !ok || movie != 0 {
		t.Fatalf("media 1024 maps to movie=%d ok=%v", movie, ok)
	}
}
