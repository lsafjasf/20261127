package laddercheck

import "testing"

func TestParseRat(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"30000/1001", "30000/1001"},
		{"30", "30"},
		{"0.5", "1/2"},
		{"", "0"},
	}
	for _, tc := range cases {
		r, err := ParseRat(tc.in)
		if err != nil {
			t.Fatalf("ParseRat(%q): %v", tc.in, err)
		}
		if got := r.String(); got != tc.want {
			t.Errorf("ParseRat(%q)=%s want %s", tc.in, got, tc.want)
		}
	}
}

func TestRatArithmeticExact(t *testing.T) {
	a := NewRat(1, 30)
	b := NewRat(60, 1).Mul(a) // 60 frames
	if b.Cmp(NewRat(2, 1)) != 0 {
		t.Fatalf("60 frames = %s, want 2", b)
	}
	// NTSC 60-frame GOP is exactly 2002/1000 seconds.
	fps := NewRat(30000, 1001)
	gop := NewRat(60, 1).Mul(NewRat(fps.D, fps.N))
	if gop.String() != "1001/500" {
		t.Fatalf("ntsc gop = %s want 1001/500 (2.002)", gop)
	}
	if frames := ratFrames(gop, fps); frames != 60 {
		t.Fatalf("ratFrames=%d want 60", frames)
	}
}

func TestGridDistance(t *testing.T) {
	origin := ZeroRat()
	step := NewRat(2, 1)
	k, d := GridDistance(NewRat(4033, 1000), origin, step) // 4.033 -> nearest 4
	if k != 2 {
		t.Fatalf("k=%d want 2", k)
	}
	if d.Cmp(NewRat(33, 1000)) != 0 {
		t.Fatalf("distance=%s want 33/1000", d)
	}
}

func TestRatJSONRoundTrip(t *testing.T) {
	var r Rat
	if err := r.UnmarshalJSON([]byte(`"30000/1001"`)); err != nil {
		t.Fatal(err)
	}
	if r.Cmp(NewRat(30000, 1001)) != 0 {
		t.Fatalf("unmarshal %s", r)
	}
	out, err := r.MarshalJSON()
	if err != nil || string(out) != `"30000/1001"` {
		t.Fatalf("marshal=%s err=%v", out, err)
	}
}
