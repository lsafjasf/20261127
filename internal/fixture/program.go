// Package fixture builds a deterministic 5.1 program with a known LFE leak.
package fixture

import (
	"math"
	"math/rand"

	"lfecheck/internal/audio"
)

const (
	LeakStartSec = 0.95
	LeakEndSec   = 2.05
	LeakGain     = 0.28
	LeakDelay    = 13
)

// Program returns the demo signal. Channels follow the 5.1 WAV order:
// FL, FR, FC, LFE, BL, BR.
func Program(sampleRate int) *audio.Buffer {
	duration := 4.2
	frames := int(math.Round(duration * float64(sampleRate)))
	channels := make([][]float64, 6)
	for i := range channels {
		channels[i] = make([]float64, frames)
	}
	rng := rand.New(rand.NewSource(20261127))
	lfe := make([]float64, frames)
	leakStart := int(math.Round(LeakStartSec * float64(sampleRate)))
	leakEnd := int(math.Round(LeakEndSec * float64(sampleRate)))
	fade := float64(sampleRate) * 0.020
	for i := 0; i < frames; i++ {
		t := float64(i) / float64(sampleRate)
		env := lfeEnvelope(i, leakStart, leakEnd, fade, sampleRate)
		if env > 0 {
			lfe[i] = 0.46 * env *
				(0.55*math.Sin(2*math.Pi*35*t) +
					0.30*math.Sin(2*math.Pi*55*t+0.4) +
					0.15*math.Sin(2*math.Pi*82*t+1.1))
		}
		channels[3][i] = lfe[i] + noise(rng, 2e-5)
	}
	for i := 0; i < frames; i++ {
		t := float64(i) / float64(sampleRate)
		dialogue := centerDialogue(t) + noise(rng, 8e-5)
		channels[2][i] = dialogue
		if i >= LeakDelay && i-LeakDelay < len(lfe) {
			channels[2][i] += LeakGain * lfe[i-LeakDelay]
		}
		channels[0][i] = 0.025*math.Sin(2*math.Pi*48*t) + noise(rng, 4e-5)
		channels[1][i] = 0.022*math.Sin(2*math.Pi*52*t+0.7) + noise(rng, 4e-5)
		channels[4][i] = 0.018*math.Sin(2*math.Pi*61*t+1.3) + noise(rng, 4e-5)
		channels[5][i] = 0.016*math.Sin(2*math.Pi*67*t+2.1) + noise(rng, 4e-5)
	}
	return &audio.Buffer{
		Format: audio.Format{
			SampleRate:  sampleRate,
			Channels:    6,
			Bits:        32,
			Encoding:    audio.Float,
			ChannelMask: audio.DefaultChannelMask(6),
		},
		Samples: channels,
	}
}

func lfeEnvelope(i, start, end int, fade float64, sampleRate int) float64 {
	if i < start || i >= end {
		return 0
	}
	attack := math.Min(1, float64(i-start)/fade)
	release := math.Min(1, float64(end-i)/fade)
	env := math.Min(attack, release)
	t := float64(i) / float64(sampleRate)
	return env * (0.75 + 0.25*math.Sin(2*math.Pi*2.5*t))
}

func centerDialogue(t float64) float64 {
	segments := [][2]float64{{0.45, 1.30}, {1.55, 2.75}, {3.10, 3.85}}
	env := 0.0
	for _, seg := range segments {
		if t < seg[0] || t >= seg[1] {
			continue
		}
		attack := math.Min(1, (t-seg[0])/0.015)
		release := math.Min(1, (seg[1]-t)/0.020)
		syllable := 0.65 + 0.35*math.Abs(math.Sin(2*math.Pi*3.7*(t-seg[0])))
		env = math.Min(attack, release) * syllable
	}
	if env == 0 {
		return 0
	}
	return 0.105 * env *
		(0.55*math.Sin(2*math.Pi*220*t) +
			0.28*math.Sin(2*math.Pi*440*t+0.3) +
			0.12*math.Sin(2*math.Pi*880*t+0.9) +
			0.05*math.Sin(2*math.Pi*1320*t+1.7))
}

func noise(rng *rand.Rand, amplitude float64) float64 {
	return amplitude * (2*rng.Float64() - 1)
}
