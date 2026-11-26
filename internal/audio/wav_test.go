package audio_test

import (
	"math"
	"testing"

	"lfecheck/internal/audio"
)

func TestWAVRoundTripAllSupportedEncodings(t *testing.T) {
	cases := []struct {
		name string
		enc  audio.Encoding
		bits int
		tol  float64
	}{
		{"pcm8", audio.PCM, 8, 1.0 / 127},
		{"pcm16", audio.PCM, 16, 1.0 / 32767},
		{"pcm24", audio.PCM, 24, 1.0 / 8388607},
		{"pcm32", audio.PCM, 32, 1.0 / 2147483647},
		{"float32", audio.Float, 32, 1e-6},
		{"float64", audio.Float, 64, 1e-12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format := audio.Format{
				SampleRate:  48000,
				Channels:    6,
				Bits:        tc.bits,
				Encoding:    tc.enc,
				ChannelMask: audio.DefaultChannelMask(6),
			}
			samples := make([][]float64, format.Channels)
			for ch := range samples {
				samples[ch] = make([]float64, 257)
				for i := range samples[ch] {
					samples[ch][i] = 0.7 * math.Sin(float64(i+ch*7)*0.13)
				}
			}
			encoded, err := audio.EncodeWAV(&audio.Buffer{Format: format, Samples: samples})
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := audio.DecodeWAV(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Format.Channels != 6 || len(decoded.Samples) != 6 {
				t.Fatalf("decoded format %+v", decoded.Format)
			}
			names := audio.ChannelNames(decoded.Format)
			want := []string{"FL", "FR", "FC", "LFE", "BL", "BR"}
			for i := range want {
				if names[i] != want[i] {
					t.Fatalf("names = %v, want %v", names, want)
				}
			}
			for ch := range samples {
				for i := range samples[ch] {
					if math.Abs(samples[ch][i]-decoded.Samples[ch][i]) > tc.tol {
						t.Fatalf("ch=%d i=%d got %g want %g", ch, i, decoded.Samples[ch][i], samples[ch][i])
					}
				}
			}
		})
	}
}
