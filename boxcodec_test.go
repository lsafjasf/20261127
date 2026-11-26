package aperturemap

import "testing"

func TestCLAPPayloadRoundTripNegativeOffset(t *testing.T) {
	original := CleanAperture{
		Width:          MustRatio(1920, 1),
		Height:         MustRatio(1080, 1),
		VerticalOffset: MustRatio(-4, 1),
	}
	payload, err := original.MarshalCLAPPayload()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCLAPPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Width.Equal(original.Width) || !parsed.Height.Equal(original.Height) {
		t.Fatalf("dimensions = %s x %s", parsed.Width, parsed.Height)
	}
	if !parsed.VerticalOffset.Equal(original.VerticalOffset) {
		t.Fatalf("vertical offset = %s, want -4", parsed.VerticalOffset)
	}
}

func TestPASPPayloadParse(t *testing.T) {
	payload := []byte{0, 0, 0, 64, 0, 0, 0, 45} // 64:45
	par, err := ParsePASPPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !par.Equal(MustRatio(64, 45)) {
		t.Fatalf("PAR = %s, want 64/45", par)
	}

	encoded, err := MarshalPASPPayload(MustRatio(64, 45))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(payload) {
		t.Fatalf("encoded PASP = %x, want %x", encoded, payload)
	}
}

func TestBoxPayloadLengthRejected(t *testing.T) {
	if _, err := ParseCLAPPayload(make([]byte, 31)); err == nil {
		t.Fatal("short clap payload accepted")
	}
	if _, err := ParsePASPPayload(make([]byte, 7)); err == nil {
		t.Fatal("short pasp payload accepted")
	}
}
