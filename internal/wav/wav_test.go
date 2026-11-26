package wav

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPCM16RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.wav")
	in := &Audio{SampleRate: 48000, Channels: 2, Frames: 3}
	in.Samples = [][]float64{{0, 0.5, -0.5}, {1, -1, 0.25}}
	if err := WritePCM16(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.SampleRate != 48000 || out.Channels != 2 || out.Frames != 3 {
		t.Fatalf("bad format: %+v", out)
	}
	if got := out.Samples[1][0]; got < 0.9999 {
		t.Fatalf("full scale not retained: %f", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
