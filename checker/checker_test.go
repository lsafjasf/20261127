package checker

import (
	"testing"

	"mediaqacontract/contract"
	"mediaqacontract/media"
)

func mustContract(t *testing.T, data string) *contract.Contract {
	t.Helper()
	c, err := contract.Load([]byte(data))
	if err != nil {
		t.Fatalf("contract.Load: %v", err)
	}
	return c
}

const testContract = `{
  "schemaVersion": "1.0",
  "platform": "TestPlatform",
  "contractId": "test-v1",
  "containers": ["mp4"],
  "video": {
    "required": true,
    "codecs": ["h264"],
    "frameRate": {"allowed": [{"num": 30000, "den": 1001}], "tolerance": 0.01},
    "bitrate": {"minBps": 5000000, "maxBps": 20000000}
  },
  "audio": {
    "required": true,
    "minStreams": 1,
    "maxStreams": 2,
    "codecs": ["aac"],
    "channelLayouts": ["stereo"],
    "bitrate": {"minBps": 128000}
  },
  "subtitles": {
    "required": true,
    "codecs": ["mov_text"],
    "languages": ["eng"]
  },
  "filename": {
    "regex": "^[A-Z]{2}-[0-9]{4}-.*\\.mp4$",
    "extensions": ["mp4"],
    "maxLength": 80
  }
}`

func validProbe() *media.Probe {
	return &media.Probe{
		Path:      "/in/US-0001-title.mp4",
		Container: "mp4",
		Video: []media.VideoStream{{
			Codec:     "h264",
			FrameRate: media.Fraction{Num: 30000, Den: 1001},
			BPS:       8000000,
		}},
		Audio: []media.AudioStream{{
			Codec: "aac", ChannelLayout: "stereo", Channels: 2, BPS: 192000,
		}},
		Subtitles: []media.SubtitleStream{{Codec: "mov_text", Language: "eng"}},
	}
}

func findItem(items []ItemResult, id string) ItemResult {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return ItemResult{ID: id, Status: "MISSING"}
}

func TestVerifyAllPass(t *testing.T) {
	report := Verify(mustContract(t, testContract), validProbe())
	if report.Overall != StatusPass {
		t.Fatalf("overall = %s, want PASS", report.Overall)
	}
	pass, fail, unknown := report.Counts()
	if fail != 0 || unknown != 0 || pass == 0 {
		t.Fatalf("counts pass=%d fail=%d unknown=%d", pass, fail, unknown)
	}
}

func TestFrameRateFailure(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	p.Video[0].FrameRate = media.Fraction{Num: 60, Den: 1}

	item := findItem(Verify(c, p).Items, "video.frameRate")
	if item.Status != StatusFail {
		t.Fatalf("frame rate status = %s, want FAIL (%s)", item.Status, item.Message)
	}
}

func TestFrameRateTolerancePass(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	// 29.971 fps vs 29.970 target: within the 0.01 tolerance.
	p.Video[0].FrameRate = media.Fraction{Num: 29971, Den: 1000}

	item := findItem(Verify(c, p).Items, "video.frameRate")
	if item.Status != StatusPass {
		t.Fatalf("frame rate status = %s, want PASS", item.Status)
	}
}

func TestMissingBitrateIsUnknownNotFail(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	p.Video[0].BPS = 0

	report := Verify(c, p)
	if report.Overall != StatusUnknown {
		t.Fatalf("overall = %s, want UNKNOWN", report.Overall)
	}
	item := findItem(report.Items, "video.bitrate")
	if item.Status != StatusUnknown {
		t.Fatalf("video bitrate status = %s, want UNKNOWN", item.Status)
	}
}

func TestMissingRequiredSubtitleFails(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	p.Subtitles = nil

	item := findItem(Verify(c, p).Items, "subtitles.presence")
	if item.Status != StatusFail {
		t.Fatalf("subtitle presence = %s, want FAIL", item.Status)
	}
}

func TestUnknownSubtitleLanguageIsUndecidable(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	p.Subtitles[0].Language = ""

	item := findItem(Verify(c, p).Items, "subtitles.language")
	if item.Status != StatusUnknown {
		t.Fatalf("subtitle language = %s, want UNKNOWN", item.Status)
	}
}

func TestChannelLayoutFailureAndCountFallback(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	// No layout name, but 6 channels: canonicalizes to 5.1, which fails stereo.
	p.Audio[0].ChannelLayout = ""
	p.Audio[0].Channels = 6

	item := findItem(Verify(c, p).Items, "audio.channelLayout")
	if item.Status != StatusFail || item.Actual != "5.1" {
		t.Fatalf("layout item = %+v, want FAIL actual 5.1", item)
	}

	// Zero layout information must be UNKNOWN, not a guessed failure.
	p.Audio[0].Channels = 0
	item = findItem(Verify(c, p).Items, "audio.channelLayout")
	if item.Status != StatusUnknown {
		t.Fatalf("layout item = %+v, want UNKNOWN", item)
	}
}

func TestAudioStreamCountBounds(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	p.Audio = append(p.Audio, p.Audio[0], p.Audio[0])

	item := findItem(Verify(c, p).Items, "audio.streams")
	if item.Status != StatusFail {
		t.Fatalf("audio stream count = %s, want FAIL", item.Status)
	}
}

func TestFilenameRules(t *testing.T) {
	c := mustContract(t, testContract)

	p := validProbe()
	p.Path = "/in/bad name.mov"
	item := findItem(Verify(c, p).Items, "filename")
	if item.Status != StatusFail {
		t.Fatalf("filename = %s, want FAIL", item.Status)
	}

	p = validProbe()
	p.Path = "/in/US-0002-" + string(make([]byte, 90)) + ".mp4"
	item = findItem(Verify(c, p).Items, "filename")
	if item.Status != StatusFail {
		t.Fatalf("long filename = %s, want FAIL", item.Status)
	}
}

func TestFailDominatesUnknown(t *testing.T) {
	c := mustContract(t, testContract)
	p := validProbe()
	p.Video[0].BPS = 0         // UNKNOWN item
	p.Video[0].Codec = "mpeg4" // FAIL item

	if got := Verify(c, p).Overall; got != StatusFail {
		t.Fatalf("overall = %s, want FAIL dominating UNKNOWN", got)
	}
}
