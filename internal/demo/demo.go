package demo

import (
	"clipforensic/internal/wav"
	"math"
	"os"
	"path/filepath"
)

const Rate = 48000

type TruthWindow struct {
	Name     string  `json:"name"`
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
	Kind     string  `json:"kind"`
}

func Music() *wav.Audio {
	dur := 2.4
	frames := int(dur * Rate)
	a := &wav.Audio{SampleRate: Rate, Channels: 2, Frames: frames, Samples: make([][]float64, 2)}
	for c := range a.Samples {
		a.Samples[c] = make([]float64, frames)
	}
	clipWindows := [][2]float64{{0.46, 0.80}, {1.32, 1.76}}
	for i := 0; i < frames; i++ {
		t := float64(i) / Rate
		chord := 0.20*math.Sin(2*math.Pi*130.81*t) +
			0.18*math.Sin(2*math.Pi*261.63*t) +
			0.14*math.Sin(2*math.Pi*329.63*t) +
			0.11*math.Sin(2*math.Pi*392.00*t)
		envelope := 0.72 + 0.28*math.Sin(2*math.Pi*1.7*t+0.4)
		v := chord * envelope
		for _, hit := range []float64{0.50, 1.38} {
			dt := t - hit
			if dt >= 0 && dt < 0.28 {
				pitch := 72*math.Exp(-18*dt) + 46
				v += 0.42 * math.Exp(-13*dt) * math.Sin(2*math.Pi*pitch*dt)
				v += 0.10 * math.Exp(-80*dt) * math.Sin(2*math.Pi*2400*dt)
			}
		}
		drive := 1.0
		for _, w := range clipWindows {
			if t >= w[0] && t <= w[1] {
				edge := math.Min((t-w[0])/0.012, (w[1]-t)/0.012)
				drive = 1.0 + 3.4*clamp01(edge)
			}
		}
		v = hardClip(v*drive, 1.0)
		a.Samples[0][i] = v
		a.Samples[1][i] = v*0.992 + 0.008*math.Sin(2*math.Pi*523.25*t)
	}
	return a
}

func Guitar() *wav.Audio {
	dur := 2.55
	frames := int(dur * Rate)
	a := &wav.Audio{SampleRate: Rate, Channels: 2, Frames: frames, Samples: make([][]float64, 2)}
	for c := range a.Samples {
		a.Samples[c] = make([]float64, frames)
	}
	freqs := []float64{164.81, 196.00, 220.00, 246.94, 329.63, 246.94, 196.00}
	starts := make([]float64, len(freqs))
	for i := range freqs {
		starts[i] = 0.16 + float64(i)*0.34
	}
	lfsr := uint32(0xA17C9E)
	for i := 0; i < frames; i++ {
		t := float64(i) / Rate
		v := 0.0
		for n, f := range freqs {
			dt := t - starts[n]
			if dt < 0 || dt > 0.34 {
				continue
			}
			attack := clamp01(dt / 0.008)
			body := math.Exp(-4.8 * dt)
			for h := 1; h <= 24; h++ {
				amp := 0.18 / math.Pow(float64(h), 0.62)
				decay := math.Exp(-float64(h-1) * 0.24 * dt)
				v += attack * body * amp * decay * math.Sin(2*math.Pi*f*float64(h)*dt+float64(h)*0.31)
			}
			if dt < 0.025 {
				lfsr ^= lfsr << 13
				lfsr ^= lfsr >> 17
				lfsr ^= lfsr << 5
				noise := float64(lfsr&0xffff)/32768 - 1
				v += 0.10 * attack * math.Exp(-120*dt) * noise
			}
		}
		v = math.Tanh(4.6*v) / math.Tanh(4.6)
		v *= 0.86
		a.Samples[0][i] = v
		a.Samples[1][i] = v*0.985 + 0.01*math.Sin(2*math.Pi*110*t)
	}
	return a
}

func Truth() []TruthWindow {
	return []TruthWindow{
		{Name: "music_limiter_clip.wav", StartSec: 0.46, EndSec: 0.80, Kind: "mastering_limiter_clip"},
		{Name: "music_limiter_clip.wav", StartSec: 1.32, EndSec: 1.76, Kind: "mastering_limiter_clip"},
		{Name: "distorted_guitar.wav", StartSec: 0.16, EndSec: 2.36, Kind: "artistic_distorted_guitar_not_incident"},
	}
}

func WriteFixtures(dir string) (musicPath, guitarPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	musicPath = filepath.Join(dir, "music_limiter_clip.wav")
	guitarPath = filepath.Join(dir, "distorted_guitar.wav")
	if err := wav.WritePCM16(musicPath, Music()); err != nil {
		return "", "", err
	}
	if err := wav.WritePCM16(guitarPath, Guitar()); err != nil {
		return "", "", err
	}
	return musicPath, guitarPath, nil
}

func hardClip(v, ceiling float64) float64 {
	if v > ceiling {
		return ceiling
	}
	if v < -ceiling {
		return -ceiling
	}
	return v
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
