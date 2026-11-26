package observermap

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want, tol float64, label string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.5f, want %.5f (tol %.5f)", label, got, want, tol)
	}
}

func TestParseHexRGB(t *testing.T) {
	cases := []struct {
		in      string
		r, g, b uint8
	}{
		{"#FF0000", 255, 0, 0},
		{"00ff00", 0, 255, 0},
		{"#00F", 0, 0, 255},
		{"abc", 170, 187, 204},
	}
	for _, c := range cases {
		r, g, b, err := ParseHexRGB(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if r != c.r || g != c.g || b != c.b {
			t.Errorf("%s -> (%d,%d,%d), want (%d,%d,%d)", c.in, r, g, b, c.r, c.g, c.b)
		}
	}
	if _, _, _, err := ParseHexRGB("#12"); err == nil {
		t.Error("expected error for 2-digit hex")
	}
}

func TestSRGBRedToXYZ(t *testing.T) {
	// IEC 61966-2-1 D65 reference values, Y=100 scale.
	got := SRGBToXYZ(255, 0, 0)
	approx(t, got.X, 41.2456, 0.02, "red X")
	approx(t, got.Y, 21.2673, 0.02, "red Y")
	approx(t, got.Z, 1.9334, 0.02, "red Z")

	white := SRGBToXYZ(255, 255, 255)
	approx(t, white.Y, 100, 0.01, "white Y")
}

func TestDeltaE2000SharmaVectors(t *testing.T) {
	// First six published Sharma/Wu/Dalal (2005) CIEDE2000 test pairs.
	type pair struct {
		l1, a1, b1, l2, a2, b2, want float64
	}
	cases := []pair{
		{50, 2.6772, -79.7751, 50, 0.0, -82.7485, 2.0425},
		{50, 3.1571, -77.2803, 50, 0.0, -82.7485, 2.8615},
		{50, 2.8361, -74.0200, 50, 0.0, -82.7485, 3.4412},
		{50, -1.3802, -84.2814, 50, 0.0, -82.7485, 1.0},
		{50, -1.1848, -84.8006, 50, 0.0, -82.7485, 1.0},
		{50, -0.9009, -85.5211, 50, 0.0, -82.7485, 1.0},
	}
	for i, c := range cases {
		got := DeltaE2000(Lab{c.l1, c.a1, c.b1}, Lab{c.l2, c.a2, c.b2})
		approx(t, got, c.want, 0.002, "Sharma vector")
		_ = i
	}
}

func TestDeltaE2000SymmetryAndZero(t *testing.T) {
	a := Lab{62.5, 12.3, -40.2}
	if d := DeltaE2000(a, a); d > 1e-12 {
		t.Errorf("identical colours ΔE00=%g, want 0", d)
	}
	b := Lab{50, -30, 20}
	d1 := DeltaE2000(a, b)
	d2 := DeltaE2000(b, a)
	if math.Abs(d1-d2) > 1e-10 {
		t.Errorf("ΔE00 not symmetric: %f vs %f", d1, d2)
	}
}

func TestChromaticity(t *testing.T) {
	if _, _, err := (XYZ{0, 0, 0}).Chromaticity(); err == nil {
		t.Error("expected error for black XYZ chromaticity")
	}
	x, y, err := WhiteD65Ref().Chromaticity()
	if err != nil {
		t.Fatal(err)
	}
	approx(t, x, 0.3127, 0.0003, "D65 x")
	approx(t, y, 0.3290, 0.0003, "D65 y")
}

// WhiteD65Ref returns the standard textbook D65 white on the Y=100 scale.
func WhiteD65Ref() XYZ { return XYZ{95.047, 100, 108.883} }
