package webm

import "testing"

func TestParseVP9KeyframeSize(t *testing.T) {
	payload := []byte{
		0x82, 0x49, 0x83, 0x42, 0x00, 0x05, 0xf0, 0x03,
		0xf6, 0x00, 0x38, 0x24,
	}
	info, err := ParseCodecHeader("VP9", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Keyframe || info.Width != 96 || info.Height != 64 {
		t.Fatalf("unexpected VP9 header: %+v", info)
	}
}

func TestParseVP8KeyframeSize(t *testing.T) {
	payload := make([]byte, 10)
	payload[3], payload[4], payload[5] = 0x9d, 0x01, 0x2a
	payload[6] = 80
	payload[8] = 60
	info, err := ParseCodecHeader("VP8", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Keyframe || info.Width != 80 || info.Height != 60 {
		t.Fatalf("unexpected VP8 header: %+v", info)
	}
}

func TestParseVP9RejectsBadSync(t *testing.T) {
	payload := []byte{0x82, 0x00, 0x00, 0x00, 0x00, 0x05, 0xf0, 0x03, 0xf6}
	if _, err := ParseCodecHeader("VP9", payload); err == nil {
		t.Fatal("expected invalid sync error")
	}
}
