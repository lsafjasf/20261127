package analyze

import (
	"clipforensic/internal/wav"
	"math"
	"testing"
)

func TestCompareFindsNewDistributionClipping(t *testing.T) {
	const rate = 48000
	const n = rate
	master := &wav.Audio{SampleRate: rate, Channels: 1, Frames: n, Samples: [][]float64{make([]float64, n)}}
	dist := &wav.Audio{SampleRate: rate, Channels: 1, Frames: n, Samples: [][]float64{make([]float64, n)}}
	for i := 0; i < n; i++ {
		master.Samples[0][i] = 0.45 * math.Sin(2*math.Pi*440*float64(i)/rate)
		if i >= 96 {
			v := 2.8 * master.Samples[0][i-96]
			dist.Samples[0][i] = math.Max(-0.99997, math.Min(0.99997, v))
		}
	}
	lag := AlignLag(master, dist)
	if lag > -64 || lag < -128 {
		t.Fatalf("expected lag near -96, got %d", lag)
	}
	rep, err := Compare(master, dist, "master.wav", "decoded.wav", DefaultOptions(rate))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Files[0].Candidates) == 0 {
		t.Fatalf("expected distribution clip candidate")
	}
	c := rep.Files[0].Candidates[0]
	if c.Comparison == nil || c.Comparison.ReferenceHadPlateau {
		t.Fatalf("expected comparison showing no master plateau: %+v", c.Comparison)
	}
	if c.Comparison.Verdict == "" || c.Classification != "distribution_encode_or_transcode_new_clip" {
		t.Fatalf("unexpected verdict/classification: %s / %s", c.Comparison.Verdict, c.Classification)
	}
}
