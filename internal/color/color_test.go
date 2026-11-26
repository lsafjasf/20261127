package color

import (
	"math"
	"testing"
)

func TestSRGBRoundTrip(t *testing.T) {
	for _, v := range []float64{0.001, 0.01, 0.18, 0.5, 0.9, 1.0} {
		got := Decode(Encode(v))
		if math.Abs(got-v) > 1e-12 {
			t.Fatalf("round trip %v: got %v", v, got)
		}
	}
}

func TestDecodeEncodeEndpoints(t *testing.T) {
	if Encode(-0.5) != 0 || Encode(2) != 1 {
		t.Fatal("Encode must clamp to [0,1]")
	}
	if Decode(0) != 0 {
		t.Fatal("Decode(0) must be 0")
	}
}

func TestHueDelta(t *testing.T) {
	cases := []struct{ a, b, want float64 }{
		{10, 20, 10},
		{350, 10, 20},
		{0, 180, 180},
	}
	for _, c := range cases {
		if got := HueDelta(c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("HueDelta(%v,%v)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestHueAchromatic(t *testing.T) {
	if _, ok := HueDeg(0.3, 0.3, 0.3); ok {
		t.Fatal("gray pixel must report no hue")
	}
	if _, ok := HueDeg(0.1, 0.5, 0.9); !ok {
		t.Fatal("colored pixel must report a hue")
	}
}
