package hdrswitch

import "testing"

func TestSEIUnitConversions(t *testing.T) {
	if got := SEIPrimaryCoordinate(35400); absFloat(got-0.708) > 1e-9 {
		t.Fatalf("red x = %v", got)
	}
	if code := QuantizeSEIPrimaryCoordinate(0.708); code != 35400 {
		t.Fatalf("red code = %d", code)
	}
	if got := SEILuminanceNits(10_000_000); got != 1000 {
		t.Fatalf("mastering max = %v nits", got)
	}
	if code := QuantizeSEILuminance(0.005); code != 50 {
		t.Fatalf("min luminance code = %d", code)
	}
}

func TestPQEndpointsAndHLGGamma(t *testing.T) {
	if got := PQLuminanceNits(0); got != 0 {
		t.Fatalf("PQ zero = %v", got)
	}
	if got := PQLuminanceNits(1); got != 10000 {
		t.Fatalf("PQ one = %v", got)
	}
	if got := PQLuminanceNits(0.5); got <= 90 || got >= 110 {
		t.Fatalf("PQ 0.5 should be about 92 nits in common approximations, got %v", got)
	}
	if got := HLGSystemGamma(1000); absFloat(got-1.2) > 1e-12 {
		t.Fatalf("HLG gamma at 1000 nits = %v", got)
	}
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
