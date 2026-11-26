package iccproof

import "testing"

func TestDeltaE00KnownSharmaPairs(t *testing.T) {
	cases := []struct {
		a, b Lab
		want float64
	}{
		{Lab{50, 2.6772, -79.7751}, Lab{50, 0, -82.7485}, 2.0425},
		{Lab{50, 3.1571, -77.2803}, Lab{50, 0, -82.7485}, 2.8615},
		{Lab{50, 2.8361, -74.0200}, Lab{50, 0, -82.7485}, 3.4412},
		{Lab{50, -1.3802, -84.2814}, Lab{50, 0, -82.7485}, 1.0000},
		{Lab{60.2574, -34.0099, 36.2677}, Lab{60.4626, -34.1751, 39.4387}, 1.2644},
		{Lab{63.0109, -31.0961, -5.8663}, Lab{62.8187, -29.7946, -4.0864}, 1.2630},
	}
	for i, tc := range cases {
		got := DeltaE00(tc.a, tc.b)
		if diff := got - tc.want; diff < -0.01 || diff > 0.01 {
			t.Fatalf("case %d DeltaE00=%.4f want %.4f", i, got, tc.want)
		}
	}
}

func TestDeltaE00ZeroForSameColor(t *testing.T) {
	c := Lab{L: 42, A: 13, B: -27}
	if got := DeltaE00(c, c); got != 0 {
		t.Fatalf("same color deltaE=%v", got)
	}
}
