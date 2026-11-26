package color

import (
	"math"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	colors := []RGB{
		{R: 0, G: 0, B: 0},
		{R: 1, G: 1, B: 1},
		{R: 1, G: 0, B: 0},
		{R: 0.75, G: 0.56, B: 0.43},
	}
	for _, matrix := range []Matrix{BT601, BT709} {
		for _, want := range colors {
			y, u, v := Encode(matrix, want.R, want.G, want.B)
			r, g, b := Decode(matrix, y, u, v)
			if maxDelta(r, want.R) > 1e-9 ||
				maxDelta(g, want.G) > 1e-9 ||
				maxDelta(b, want.B) > 1e-9 {
				t.Fatalf("%s round trip: got %.9f %.9f %.9f want %+v", matrix, r, g, b, want)
			}
		}
	}
}

func TestFromFFprobeAliases(t *testing.T) {
	tests := map[string]Matrix{
		"bt470bg": BT601, "smpte170m": BT601, "bt709": BT709,
		"": "", "bt2020nc": "",
	}
	for input, want := range tests {
		if got := FromFFprobe(input); got != want {
			t.Errorf("FromFFprobe(%q) = %q, want %q", input, got, want)
		}
	}
}

func maxDelta(a, b float64) float64 {
	return math.Abs(a - b)
}
