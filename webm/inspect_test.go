package webm

import (
	"os"
	"testing"
)

func TestInspectRealAlphaFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/alpha-ok.webm")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Inspect(data, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict.Status != "pass" {
		t.Fatalf("status=%s findings=%+v", rep.Verdict.Status, rep.Findings)
	}
	track := rep.Tracks[0]
	if track.CodecID != "V_VP9" || track.AlphaMode != 1 || !track.AlphaModePresent {
		t.Fatalf("bad track alpha/codec: %+v", track)
	}
	if track.PixelWidth != 96 || track.PixelHeight != 64 || track.VisibleWidth != 96 {
		t.Fatalf("bad dimensions: %+v", track)
	}
	if track.Bitstream == nil || track.Bitstream.Width != 96 || track.Bitstream.Height != 64 {
		t.Fatalf("bad bitstream: %+v", track.Bitstream)
	}
	if track.FrameCount != 12 || track.Alpha.FramesWithAdditional != 12 {
		t.Fatalf("bad counts frames=%d alpha=%d", track.FrameCount, track.Alpha.FramesWithAdditional)
	}
	if rep.Timing.Gaps != 0 || rep.Timing.NonMonotonic != 0 || rep.Timing.Duplicates != 0 {
		t.Fatalf("bad timing: %+v", rep.Timing)
	}
	if got := rep.Timing.DefaultFrameRate; got < 11.9 || got > 12.1 {
		t.Fatalf("frame rate %.3f", got)
	}

	frames, err := ExtractVideoFrames(data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 12 || len(frames[0].Payload) == 0 || len(frames[0].AlphaPayload) == 0 {
		t.Fatalf("bad extracted frames: %d first=%+v", len(frames), frames[0])
	}
}
