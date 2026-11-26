package colorcompare

import (
	"math"
	"testing"
)

func TestAnalyzeSamplesFindsDarkOrderingReversal(t *testing.T) {
	analysis, err := AnalyzeSamples([]Sample{
		{ID: "dark-small", A: RGB24{0x0E, 0x0C, 0x25}, B: RGB24{0x0E, 0x0C, 0x26}},
		{ID: "light-small", A: RGB24{0xDE, 0xD3, 0x76}, B: RGB24{0xDF, 0xD3, 0x78}},
	})
	if err != nil {
		t.Fatal(err)
	}

	dark := analysis.Samples[0]
	light := analysis.Samples[1]
	if dark.EuclideanRGB >= light.EuclideanRGB {
		t.Fatalf("expected dark pair to be closer in RGB: %.6f >= %.6f",
			dark.EuclideanRGB, light.EuclideanRGB)
	}
	if dark.DeltaE2000 <= light.DeltaE2000 {
		t.Fatalf("expected dark pair to be farther in CIEDE2000: %.6f <= %.6f",
			dark.DeltaE2000, light.DeltaE2000)
	}
	if dark.EuclideanRank != 1 || dark.DeltaE2000Rank != 2 {
		t.Fatalf("dark ranks = RGB %d, CIEDE2000 %d; want 1, 2",
			dark.EuclideanRank, dark.DeltaE2000Rank)
	}
	if light.EuclideanRank != 2 || light.DeltaE2000Rank != 1 {
		t.Fatalf("light ranks = RGB %d, CIEDE2000 %d; want 2, 1",
			light.EuclideanRank, light.DeltaE2000Rank)
	}
	if len(analysis.Reversals) != 1 {
		t.Fatalf("got %d reversals, want 1", len(analysis.Reversals))
	}
	reversal := analysis.Reversals[0]
	if reversal.EuclideanCloser != "dark-small" || reversal.DeltaE2000Closer != "light-small" {
		t.Fatalf("unexpected reversal: %+v", reversal)
	}
	if got := math.Sqrt(dark.Components.SquaredSum()); math.Abs(got-dark.DeltaE2000) > 1e-12 {
		t.Fatalf("dark component reconstruction = %.12f, want %.12f", got, dark.DeltaE2000)
	}
}

func TestDeltaE2000ComponentRegions(t *testing.T) {
	tests := []struct {
		name      string
		a, b      RGB24
		dominant  Dimension
		minChroma float64
		expected  float64
	}{
		{
			name:      "near gray axis",
			a:         RGB24{128, 128, 128},
			b:         RGB24{128, 128, 129},
			dominant:  DimensionChroma,
			minChroma: 0.1,
			expected:  0.610221,
		},
		{
			name:      "high saturation hue",
			a:         RGB24{255, 0, 0},
			b:         RGB24{0, 255, 0},
			dominant:  DimensionHue,
			minChroma: 50,
			expected:  86.608237,
		},
		{
			name:      "lightness lower boundary",
			a:         RGB24{0, 0, 0},
			b:         RGB24{1, 0, 0},
			dominant:  DimensionChroma,
			minChroma: 0.1,
			expected:  0.400703,
		},
		{
			name:      "lightness upper boundary",
			a:         RGB24{255, 255, 255},
			b:         RGB24{254, 254, 254},
			dominant:  DimensionLightness,
			minChroma: 0,
			expected:  0.197818,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareRGB(tt.a, tt.b)
			assertFiniteDeltaE(t, result)
			if result.Dominant != tt.dominant {
				t.Fatalf("dominant = %q, want %q (components: %+v)",
					result.Dominant, tt.dominant, result.Components)
			}
			largestChroma := math.Max(result.Components.Chroma1, result.Components.Chroma2)
			if largestChroma < tt.minChroma {
				t.Fatalf("largest chroma = %.6f, want >= %.6f", largestChroma, tt.minChroma)
			}
			reverse := CompareRGB(tt.b, tt.a)
			if math.Abs(result.DeltaE2000-reverse.DeltaE2000) > 1e-12 {
				t.Fatalf("asymmetry: %.12f vs %.12f", result.DeltaE2000, reverse.DeltaE2000)
			}
			if math.Abs(result.DeltaE2000-tt.expected) > 1e-4 {
				t.Fatalf("DeltaE2000 = %.9f, want %.6f", result.DeltaE2000, tt.expected)
			}
		})
	}
}

func TestDeltaE2000LightnessEndpointsAreUnscaled(t *testing.T) {
	black := RGB24{0, 0, 0}
	white := RGB24{255, 255, 255}
	result := CompareRGB(black, white)

	if math.Abs(result.DeltaE2000-100) > 1e-4 {
		t.Fatalf("black/white DeltaE2000 = %.9f, want approximately 100", result.DeltaE2000)
	}
	if math.Abs(result.EuclideanRGB-math.Sqrt(3*255*255)) > 1e-12 {
		t.Fatalf("black/white RGB distance = %.9f, want raw 0..255 cube distance",
			result.EuclideanRGB)
	}
	assertFiniteDeltaE(t, result)
}

func TestDeltaE2000SquaredDecomposition(t *testing.T) {
	pairs := [][2]RGB24{
		{{128, 128, 128}, {128, 128, 129}},
		{{255, 0, 0}, {0, 255, 0}},
		{{0, 0, 0}, {255, 255, 255}},
		{{0x0E, 0x0C, 0x25}, {0x0E, 0x0C, 0x26}},
	}
	for _, pair := range pairs {
		result := CompareRGB(pair[0], pair[1])
		components := result.Components
		squared := components.LightnessContribution() +
			components.ChromaContribution() +
			components.HueContribution() +
			components.RotationContribution()
		if math.Abs(math.Sqrt(squared)-result.DeltaE2000) > 1e-12 {
			t.Fatalf("%s/%s component reconstruction failed: %.12f vs %.12f",
				pair[0].Hex(), pair[1].Hex(), math.Sqrt(squared), result.DeltaE2000)
		}
	}
}

func TestLabFromRGBPrimaries(t *testing.T) {
	tests := []struct {
		name string
		rgb  RGB24
		want Lab
	}{
		{"red", RGB24{255, 0, 0}, Lab{53.2408, 80.0925, 67.2032}},
		{"green", RGB24{0, 255, 0}, Lab{87.7347, -86.1827, 83.1793}},
		{"blue", RGB24{0, 0, 255}, Lab{32.2970, 79.1875, -107.8602}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LabFromRGB(tt.rgb)
			if math.Abs(got.L-tt.want.L) > 1e-3 ||
				math.Abs(got.A-tt.want.A) > 1e-3 ||
				math.Abs(got.B-tt.want.B) > 1e-3 {
				t.Fatalf("LabFromRGB(%s) = %+v, want %+v", tt.rgb.Hex(), got, tt.want)
			}
		})
	}
}

func assertFiniteDeltaE(t *testing.T, result PairComparison) {
	t.Helper()
	if math.IsNaN(result.DeltaE2000) || math.IsInf(result.DeltaE2000, 0) {
		t.Fatalf("non-finite DeltaE2000: %v", result.DeltaE2000)
	}
	if result.DeltaE2000 < 0 {
		t.Fatalf("negative DeltaE2000: %v", result.DeltaE2000)
	}
}
