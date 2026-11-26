package click

import (
	"math"
	"math/rand"
	"testing"

	"clickdetect/internal/wav"
)

const testRate = 48000

func audioFrom(x []float64) *wav.Audio {
	return &wav.Audio{SampleRate: testRate, Channels: 1, Frames: len(x), Samples: x}
}

func options() Options {
	o := DefaultOptions()
	o.WaveformBefore = 4
	o.WaveformAfter = 4
	return o
}

func TestPureToneHasNoClick(t *testing.T) {
	x := sine(testRate, 0.6, 440, 0, 0.25)
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 0 {
		t.Fatalf("pure tone produced detections: %+v", got.Detections)
	}
}

func TestDetectsSingleSampleDiscontinuity(t *testing.T) {
	x := sine(testRate, 0.6, 440, 0, 0.25)
	const at = 17003
	x[at] += 0.65
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(got.Detections), got.Detections)
	}
	d := got.Detections[0]
	if absInt(d.Sample-at) > 2 {
		t.Fatalf("click sample %d, want %d (+/-2)", d.Sample, at)
	}
	if !hasFlag(d.Flags, "sample_discontinuity") {
		t.Fatalf("missing sample_discontinuity flag: %v", d.Flags)
	}
	if len(d.Waveform.Samples) != 1 || len(d.Waveform.Samples[0]) == 0 {
		t.Fatal("waveform was not attached")
	}
}

func TestDetectsPhaseSplice(t *testing.T) {
	cut := testRate / 2
	x := make([]float64, testRate)
	for i := range x {
		phase := 2 * math.Pi * 523.25 * float64(i) / testRate
		if i >= cut {
			phase += 1.15
		}
		x[i] = 0.3 * math.Sin(phase)
	}
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(got.Detections), got.Detections)
	}
	d := got.Detections[0]
	if d.Sample != cut {
		t.Fatalf("click sample %d, want %d", d.Sample, cut)
	}
	if !hasFlag(d.Flags, "phase_jump") {
		t.Fatalf("phase splice did not report phase_jump: %+v", d.Features)
	}
}

func TestDetectsLowFrequencyStep(t *testing.T) {
	x := sine(testRate, 0.7, 440, 0, 0.2)
	cut := 22000
	for i := cut; i < len(x); i++ {
		x[i] += 0.12
	}
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(got.Detections), got.Detections)
	}
	d := got.Detections[0]
	if d.Sample != cut {
		t.Fatalf("click sample %d, want %d", d.Sample, cut)
	}
	if !hasFlag(d.Flags, "low_frequency_impulse") {
		t.Fatalf("low-frequency step was not reported: %+v", d.Features)
	}
}

func TestDetectsSmoothLowFrequencyStep(t *testing.T) {
	x := sine(testRate, 0.7, 440, 0, 0.2)
	cut := 22000
	ramp := int(0.001 * testRate)
	for i := cut; i < len(x); i++ {
		gain := 1.0
		if i-cut < ramp {
			gain = 0.5 - 0.5*math.Cos(math.Pi*float64(i-cut)/float64(ramp))
		}
		x[i] += 0.12 * gain
	}
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(got.Detections), got.Detections)
	}
	d := got.Detections[0]
	if absInt(d.Sample-cut) > 48 {
		t.Fatalf("click sample %d, want near %d", d.Sample, cut)
	}
	if !hasFlag(d.Flags, "low_frequency_impulse") {
		t.Fatalf("smooth low-frequency step was not reported: %+v", d.Features)
	}
}

func TestNormalDrumTrackIsNotFlagged(t *testing.T) {
	x := make([]float64, testRate*2)
	for _, at := range []float64{0.15, 0.55, 0.95, 1.35, 1.75} {
		addKick(x, int(at*testRate), 0.55)
	}
	for _, at := range []float64{0.35, 0.75, 1.15, 1.55} {
		addSnare(x, int(at*testRate), 0.20, 17)
	}
	for _, at := range []float64{0.25, 0.45, 0.65, 0.85, 1.05, 1.25, 1.45, 1.65, 1.85} {
		addHat(x, int(at*testRate), 0.08, int(at*1000)+5)
	}
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 0 {
		t.Fatalf("normal drums produced %d clicks: %+v", len(got.Detections), got.Detections)
	}
	if got.SuppressedTotal == 0 {
		t.Fatal("expected at least one transient to be classified by the drum guard")
	}
}

func TestClickOverDrumTrackIsStillDetected(t *testing.T) {
	x := make([]float64, testRate*2)
	for _, at := range []float64{0.15, 0.55, 0.95, 1.35, 1.75} {
		addKick(x, int(at*testRate), 0.5)
	}
	for _, at := range []float64{0.35, 0.75, 1.15, 1.55} {
		addSnare(x, int(at*testRate), 0.16, 19)
	}
	const at = 44107
	x[at] += 0.45
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(got.Detections), got.Detections)
	}
	if absInt(got.Detections[0].Sample-at) > 3 {
		t.Fatalf("click sample %d, want %d (+/-3)", got.Detections[0].Sample, at)
	}
}

func TestHardDrumAttacksAreGuarded(t *testing.T) {
	x := make([]float64, testRate)
	const kickAt = 8000
	for i := 0; i < testRate/4; i++ {
		t := float64(i) / testRate
		env := math.Exp(-t / 0.10)
		x[kickAt+i] += 0.65 * env * math.Cos(2*math.Pi*61*t)
	}
	const snareAt = 24000
	r := rand.New(rand.NewSource(29))
	for i := 0; i < testRate/8; i++ {
		t := float64(i) / testRate
		env := math.Exp(-t / 0.045)
		x[snareAt+i] += 0.24 * env * (r.NormFloat64() + 0.4*math.Sin(2*math.Pi*210*t))
	}
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 0 {
		t.Fatalf("hard drum attacks produced clicks: %+v", got.Detections)
	}
}

func TestDrumsOverSustainedMixAreNotFlagged(t *testing.T) {
	x := make([]float64, testRate*6)
	for i := range x {
		t := float64(i) / testRate
		x[i] = 0.09*math.Sin(2*math.Pi*220*t) + 0.06*math.Sin(2*math.Pi*331*t)
	}
	for _, at := range []float64{0.30, 0.70, 1.10, 1.50, 1.90, 2.30, 2.70, 3.10, 3.50, 3.90, 4.30, 4.70, 5.10, 5.50} {
		addKick(x, int(at*testRate), 0.45)
	}
	for _, at := range []float64{0.50, 0.90, 1.30, 1.70, 2.10, 2.50, 2.90, 3.30, 3.70, 4.10, 4.50, 4.90, 5.30, 5.70} {
		addSnare(x, int(at*testRate), 0.14, int(at*100)+31)
	}
	got := Analyze(audioFrom(x), options())
	if len(got.Detections) != 0 {
		t.Fatalf("drums over sustained mix produced clicks: %+v", got.Detections)
	}
}

func TestStereoClickIsMerged(t *testing.T) {
	frames := testRate / 2
	samples := make([]float64, frames*2)
	for i := 0; i < frames; i++ {
		v := 0.15 * math.Sin(2*math.Pi*330*float64(i)/testRate)
		samples[2*i] = v
		samples[2*i+1] = v
	}
	const at = 12000
	samples[2*at] += 0.5
	samples[2*at+1] += 0.5
	a := &wav.Audio{SampleRate: testRate, Channels: 2, Frames: frames, Samples: samples}
	got := Analyze(a, options())
	if len(got.Detections) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(got.Detections), got.Detections)
	}
	d := got.Detections[0]
	if len(d.Channels) != 2 || d.Channels[0] != 0 || d.Channels[1] != 1 {
		t.Fatalf("channels = %v, want [0 1]", d.Channels)
	}
	if len(d.Waveform.Samples) != 2 {
		t.Fatalf("waveform channels = %d, want 2", len(d.Waveform.Samples))
	}
}

func TestDetectsAt44100(t *testing.T) {
	const rate = 44100
	x := make([]float64, rate)
	for i := range x {
		x[i] = 0.2 * math.Sin(2*math.Pi*997*float64(i)/rate)
	}
	const at = 21873
	x[at] += 0.55
	a := &wav.Audio{SampleRate: rate, Channels: 1, Frames: len(x), Samples: x}
	got := Analyze(a, options())
	if len(got.Detections) != 1 {
		t.Fatalf("got %d detections, want 1: %+v", len(got.Detections), got.Detections)
	}
	if got.Detections[0].Sample != at {
		t.Fatalf("click sample %d, want %d", got.Detections[0].Sample, at)
	}
}

func sine(rate int, seconds, hz, phase, amp float64) []float64 {
	n := int(float64(rate) * seconds)
	x := make([]float64, n)
	for i := range x {
		x[i] = amp * math.Sin(2*math.Pi*hz*float64(i)/float64(rate)+phase)
	}
	return x
}

func addKick(x []float64, at int, amp float64) {
	attack := int(0.004 * testRate)
	decay := 0.11 * testRate
	for i := 0; at+i < len(x) && i < int(decay*6); i++ {
		t := float64(i) / testRate
		env := math.Exp(-t / 0.11)
		if i < attack {
			env *= float64(i) / float64(attack)
		}
		x[at+i] += amp * env * math.Sin(2*math.Pi*58*t)
	}
}

func addSnare(x []float64, at int, amp float64, seed int) {
	r := rand.New(rand.NewSource(int64(seed)))
	attack := int(0.0015 * testRate)
	for i := 0; at+i < len(x) && i < int(0.16*testRate); i++ {
		t := float64(i) / testRate
		env := math.Exp(-t / 0.055)
		if i < attack {
			env *= float64(i) / float64(attack)
		}
		tone := 0.55 * math.Sin(2*math.Pi*190*t)
		x[at+i] += amp * env * (0.6*r.NormFloat64() + tone)
	}
}

func addHat(x []float64, at int, amp float64, seed int) {
	r := rand.New(rand.NewSource(int64(seed)))
	attack := 38
	var previous float64
	for i := 0; at+i < len(x) && i < int(0.045*testRate); i++ {
		t := float64(i) / testRate
		env := math.Exp(-t / 0.014)
		if i < attack {
			env *= float64(i) / float64(attack)
		}
		white := r.NormFloat64()
		high := white - previous
		previous = white
		x[at+i] += amp * env * high
	}
}

func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
