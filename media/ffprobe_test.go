package media

import "testing"

const sampleFFProbe = `{
  "format": {"format_name": "mov,mp4,m4a,3gp,3g2,mj2", "bit_rate": "8000000"},
  "streams": [
    {
      "index": 0,
      "codec_type": "video",
      "codec_name": "h264",
      "avg_frame_rate": "30000/1001",
      "r_frame_rate": "30000/1001",
      "bit_rate": "8000000"
    },
    {
      "index": 1,
      "codec_type": "audio",
      "codec_name": "aac",
      "channels": 2,
      "channel_layout": "stereo",
      "bit_rate": "N/A"
    },
    {
      "index": 2,
      "codec_type": "subtitle",
      "codec_name": "mov_text",
      "tags": {"language": "eng"}
    }
  ]
}`

func TestParseFFProbeJSON(t *testing.T) {
	probe, err := ParseFFProbeJSON("/tmp/demo.mp4", []byte(sampleFFProbe))
	if err != nil {
		t.Fatalf("ParseFFProbeJSON: %v", err)
	}
	if probe.Container != "mov" {
		t.Fatalf("container = %q", probe.Container)
	}
	if len(probe.ContainerAliases) != 6 || probe.ContainerAliases[1] != "mp4" {
		t.Fatalf("container aliases = %v", probe.ContainerAliases)
	}
	if len(probe.Video) != 1 || probe.Video[0].Codec != "h264" {
		t.Fatalf("video streams = %+v", probe.Video)
	}
	if fps, _ := probe.Video[0].FrameRate.FPS(); fps < 29.96 || fps > 29.98 {
		t.Fatalf("fps = %v", fps)
	}
	if probe.Video[0].BPS != 8000000 {
		t.Fatalf("video bps = %d", probe.Video[0].BPS)
	}
	if len(probe.Audio) != 1 || probe.Audio[0].BPS != 0 {
		t.Fatalf("audio = %+v, want N/A normalized to 0", probe.Audio)
	}
	if probe.Audio[0].ChannelLayout != "stereo" {
		t.Fatalf("layout = %q", probe.Audio[0].ChannelLayout)
	}
	if len(probe.Subtitles) != 1 || probe.Subtitles[0].Language != "eng" {
		t.Fatalf("subtitles = %+v", probe.Subtitles)
	}
}

func TestParseFractionAndBPS(t *testing.T) {
	if got := parseFraction("0/0"); got != (Fraction{}) {
		t.Fatalf("0/0 = %+v", got)
	}
	if got := parseBPS("N/A"); got != 0 {
		t.Fatalf("N/A bps = %d", got)
	}
	if got := parseBPS("128000"); got != 128000 {
		t.Fatalf("bps = %d", got)
	}
}
