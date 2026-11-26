package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractVideoSmoke(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "src.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	mk := exec.CommandContext(ctx, "ffmpeg", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:rate=30",
		"-pix_fmt", "yuv420p", mp4)
	if out, err := mk.CombinedOutput(); err != nil {
		t.Skipf("cannot generate test video: %v %s", err, out)
	}

	var buf bytes.Buffer
	n, err := ExtractVideo(ctx, mp4, &buf)
	if err != nil {
		t.Fatalf("ExtractVideo: %v", err)
	}
	if n < 25 {
		t.Fatalf("extracted %d frames, want ~30", n)
	}
	dec := json.NewDecoder(&buf)
	first := map[string]any{}
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode first jsonl: %v", err)
	}
	if first["hash"] == nil || first["pts"] == nil {
		t.Fatalf("first record missing hash/pts: %v", first)
	}
}
