package diag

import (
	"math"
	"os"
	"testing"
	"time"

	"seekdiag/internal/mp4"
	"seekdiag/internal/testmedia"
)

func TestLayerAttribution(t *testing.T) {
	path := t.TempDir() + "/audio.mp4"
	if err := os.WriteFile(path, testmedia.BuildAudioMP4(), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := mp4.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	const startIndex = 56_252
	for _, tc := range []struct {
		name string
		obs  func(index int64, expected, drift float64) Observation
		want string
	}{
		{
			name: "container",
			want: VerdictContainer,
			obs: func(index int64, expected, drift float64) Observation {
				packet := expected + drift
				return Observation{AudioPTSMS: fptr(packet), DecodedAudioPTSMS: fptr(packet), PlaybackClockMS: fptr(packet)}
			},
		},
		{
			name: "decoder",
			want: VerdictDecoder,
			obs: func(index int64, expected, drift float64) Observation {
				decoded := expected + drift
				return Observation{AudioPTSMS: fptr(expected), DecodedAudioPTSMS: fptr(decoded), PlaybackClockMS: fptr(decoded)}
			},
		},
		{
			name: "clock",
			want: VerdictClock,
			obs: func(index int64, expected, drift float64) Observation {
				return Observation{AudioPTSMS: fptr(expected), DecodedAudioPTSMS: fptr(expected), PlaybackClockMS: fptr(expected + drift)}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := &ObservationSet{}
			for i := int64(0); i < 4; i++ {
				index := startIndex + i*1406
				expected := moviePTS(index)
				o := tc.obs(index, expected, float64(i*30))
				o.WallMS = float64(i * 30_000)
				o.AudioSampleIndex = iptr(index)
				set.After = append(set.After, o)
			}
			result := Analyze(f, 20*time.Minute, set, DefaultOptions())
			if result.Verdict != tc.want {
				t.Fatalf("verdict = %s, want %s; findings=%+v drift=%+v", result.Verdict, tc.want, result.Findings, result.Observations.Drift)
			}
		})
	}
}

func moviePTS(oneBasedIndex int64) float64 {
	mediaTicks := (oneBasedIndex - 1) * 1024
	return float64(mediaTicks-1024) / 48
}

func fptr(v float64) *float64 { return &v }
func iptr(v int64) *int64     { return &v }

func TestNoObservationIsInconclusive(t *testing.T) {
	path := t.TempDir() + "/audio.mp4"
	if err := os.WriteFile(path, testmedia.BuildAudioMP4(), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := mp4.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	result := Analyze(f, 20*time.Minute, nil, DefaultOptions())
	if result.Verdict != VerdictInconclusive {
		t.Fatalf("verdict = %s, want inconclusive", result.Verdict)
	}
	if result.SeekPlan == nil || math.Abs(result.SeekPlan.TargetMS-1_200_000) > 0.001 {
		t.Fatalf("bad seek plan: %+v", result.SeekPlan)
	}
}
