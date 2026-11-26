package subtitle

import "testing"

func TestParseTime(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"00:00:01,000", 1_000_000_000},
		{"01:02:03,456", (3600+120+3)*1_000_000_000 + 456_000_000},
		{"00:00:00.500", 500_000_000},
		{"01:02:03.000000001", (3600+120+3)*1_000_000_000 + 1},
		{"59.500", 59_500_000_000}, // VTT MM:SS
	}
	for _, c := range cases {
		got, err := ParseTime(c.in)
		if err != nil {
			t.Fatalf("ParseTime(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseTime(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseTimeErrors(t *testing.T) {
	for _, in := range []string{"", "12:61:00,000", "00:00,000", "00:00", "bad", "00:00:00.1234567890"} {
		if _, err := ParseTime(in); err == nil {
			t.Errorf("ParseTime(%q) expected error", in)
		}
	}
}

func TestFormatRoundTrip(t *testing.T) {
	v := int64((2*3600+3*60+4)*1_000_000_000 + 12_000_000)
	if got := FormatSRT(v); got != "02:03:04,012" {
		t.Fatalf("FormatSRT = %q", got)
	}
	if got := FormatVTT(v); got != "02:03:04.012" {
		t.Fatalf("FormatVTT = %q", got)
	}
	full := int64(1_000_000_001)
	if got := FormatESUB(full); got != "00:00:01.000000001" {
		t.Fatalf("FormatESUB = %q", got)
	}
	back, err := ParseTime(FormatESUB(full))
	if err != nil || back != full {
		t.Fatalf("ESUB round trip = %d, err %v", back, err)
	}
}

func TestValidLanguage(t *testing.T) {
	for _, ok := range []string{"zh", "en", "en-US", "zh-Hant", "pt-BR"} {
		if !ValidLanguage(ok) {
			t.Errorf("expected %q valid", ok)
		}
	}
	for _, bad := range []string{"", "e", "123", "en_"} {
		if ValidLanguage(bad) {
			t.Errorf("expected %q invalid", bad)
		}
	}
}
