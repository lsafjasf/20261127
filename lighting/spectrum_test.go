package lighting

import (
	"math"
	"testing"
)

func TestAnalyzeBandsFindsFlickerFrequency(t *testing.T) {
	cfg := DefaultConfig()
	fps := 30.0
	n := 300
	series := make([]float64, n)
	for i := range series {
		series[i] = 6 * math.Sin(2*math.Pi*10*float64(i)/fps)
	}
	bands := analyzeBands(series, fps, cfg)
	if len(bands) != 3 {
		t.Fatalf("got %d bands, want 3", len(bands))
	}
	flicker := bands[2]
	if math.Abs(flicker.PeakHz-10) > 0.2 {
		t.Fatalf("peak %.3f Hz, want 10 Hz", flicker.PeakHz)
	}
	if flicker.RelativePower < 0.9 {
		t.Fatalf("flicker relative power %.3f, want >= 0.9", flicker.RelativePower)
	}
	if want := 6 / math.Sqrt2; math.Abs(flicker.RMSLuma-want) > 0.8 {
		t.Fatalf("flicker RMS %.3f, want %.3f", flicker.RMSLuma, want)
	}
}
