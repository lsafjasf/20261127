package manifest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExecProbeWithGeneratedMedia(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}

	dir := t.TempDir()
	subtitlePath := filepath.Join(dir, "zh.srt")
	if err := os.WriteFile(subtitlePath, []byte("1\n00:00:00,000 --> 00:00:01,000\nTest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(dir, "probe.mkv")
	command := exec.Command(
		ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=64x36:d=1:r=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:d=1",
		"-i", subtitlePath,
		"-map", "0:v", "-map", "1:a", "-map", "2:s",
		"-c:v", "mpeg4", "-q:v", "2", "-pix_fmt", "yuv420p",
		"-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709",
		"-c:a", "pcm_s16le", "-c:s", "srt",
		"-metadata:s:a:0", "language=zho", "-disposition:a:0", "default",
		"-metadata:s:s:0", "language=zho", "-disposition:s:0", "default",
		mediaPath,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg fixture generation is unavailable: %v (%s)", err, output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prober := NewExecProbe(ffprobePath, ffmpegPath)
	probe, err := prober.Probe(ctx, mediaPath)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if len(probe.Streams) != 3 {
		t.Fatalf("stream count = %d, want 3", len(probe.Streams))
	}
	video, ok := probe.FirstVideo()
	if !ok {
		t.Fatal("video stream missing")
	}
	if video.ColorPrimaries != "bt709" || video.ColorTransfer != "bt709" || video.ColorSpace != "bt709" {
		t.Fatalf("unexpected color tags: %+v", video)
	}
	if frames, ok := ParsePositiveInt64(video.NBReadFrames); !ok || frames != 24 {
		t.Fatalf("nb_read_frames = %q, want 24", video.NBReadFrames)
	}
	audio, ok := probe.FirstStreamOfType("audio")
	if !ok {
		t.Fatal("audio stream missing")
	}
	subtitle, ok := probe.FirstStreamOfType("subtitle")
	if !ok {
		t.Fatal("subtitle stream missing")
	}
	audioHash, err := prober.StreamSHA256(ctx, mediaPath, audio.Index)
	if err != nil || audioHash == "" {
		t.Fatalf("audio stream hash = %q, err = %v", audioHash, err)
	}
	subtitleHash, err := prober.StreamSHA256(ctx, mediaPath, subtitle.Index)
	if err != nil || subtitleHash == "" {
		t.Fatalf("subtitle stream hash = %q, err = %v", subtitleHash, err)
	}
	if audioHash == subtitleHash {
		t.Fatal("audio and subtitle stream hashes should differ")
	}
}
