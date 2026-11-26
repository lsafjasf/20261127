package timepoint

import "testing"

func TestParseClockForms(t *testing.T) {
	cases := []struct {
		raw  string
		want uint64
	}{
		{"0", 0},
		{"1.5", 1_500_000_000},
		{"01:02", 62_000_000_000},
		{"1:02.250", 62_250_000_000},
		{"01:02:03.123456789", 3723_123456789},
		{"00:00:01,005", 1_005_000_000},
	}
	for _, tc := range cases {
		p := Parse(tc.raw)
		if !p.Ok() {
			t.Fatalf("Parse(%q) unresolved: %s", tc.raw, p.Reason)
		}
		if *p.NS != tc.want {
			t.Fatalf("Parse(%q) = %d, want %d", tc.raw, *p.NS, tc.want)
		}
	}
}

func TestParseBadAndFrameRate(t *testing.T) {
	if p := Parse("1:70"); p.Ok() {
		t.Fatal("expected bad minute to remain unresolved")
	}
	if p := Parse("not-a-time"); p.Ok() {
		t.Fatal("expected garbage to remain unresolved")
	}
	p := Parse("01:00:00:02")
	if p.Status != StatusFrameRateRequired {
		t.Fatalf("status = %q, want frame_rate_required", p.Status)
	}
	rate, err := ParseRate("24000/1001")
	if err != nil {
		t.Fatal(err)
	}
	p = Parse("00:00:00:23", ParseOptions{FPS: rate})
	if !p.Ok() {
		t.Fatalf("frame 23 should resolve at 24000/1001: %s", p.Reason)
	}
	p = Parse("00:00:02:00", ParseOptions{FPS: rate})
	if !p.Ok() || *p.NS != 2*NanosecondPerSecond {
		t.Fatalf("SMPTE seconds = %+v, want exactly 2s", p)
	}
	p = Parse("00:00:00:24", ParseOptions{FPS: rate})
	if p.Status != StatusFrameOutOfRange {
		t.Fatalf("status = %q, want frame_out_of_range", p.Status)
	}
}

func TestCutMappingBoundariesAndInterior(t *testing.T) {
	m := TimelineMap{Cuts: []Cut{{StartNS: 10, EndNS: 20}}}
	for _, ns := range []uint64{10, 20} {
		got, status, _ := m.MapNS(ns)
		if status != StatusResolved || got != 10 {
			t.Fatalf("boundary %d mapped to (%d,%q), want 10,resolved", ns, got, status)
		}
	}
	if _, status, _ := m.MapNS(15); status != StatusRemoved {
		t.Fatalf("interior status = %q, want removed", status)
	}
	got, status, _ := m.MapNS(25)
	if status != StatusResolved || got != 15 {
		t.Fatalf("after cut = (%d,%q), want 15,resolved", got, status)
	}
}

func TestOffsetAndDurationProduceUnresolved(t *testing.T) {
	duration := uint64(100)
	m := TimelineMap{OffsetNS: -1000, DurationNS: &duration}
	if _, status, _ := m.MapNS(1); status != StatusBeforeZero {
		t.Fatalf("status = %q, want before_zero", status)
	}
	m = TimelineMap{DurationNS: &duration}
	if _, status, _ := m.MapNS(101); status != StatusOutOfTargetRange {
		t.Fatalf("status = %q, want out_of_target_range", status)
	}
}
