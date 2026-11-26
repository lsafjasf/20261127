package main

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testRate = 48000

// scenario builds a stereo signal with content, digital silence, noise floor
// and a soft pause. d2MS controls the second digital silence so two files can
// differ by tens of milliseconds.
func scenario(t *testing.T, d2MS int) *Audio {
	t.Helper()
	ms := testRate / 1000
	plan := []struct {
		durMS int
		kind  string // tone, digital, noise, pause
	}{
		{50, "tone"},
		{200, "digital"},
		{200, "tone"},
		{250, "noise"},
		{200, "tone"},
		{500, "pause"},
		{200, "tone"},
		{d2MS, "digital"},
		{200, "tone"},
		{100, "digital"},
	}
	total := 0
	for _, p := range plan {
		total += p.durMS
	}
	a := &Audio{
		Path:       "synthetic.wav",
		SampleRate: testRate,
		Channels:   2,
		Bits:       32,
		Frames:     total * ms,
		Samples:    make([][]int32, 2),
	}
	for ch := range a.Samples {
		a.Samples[ch] = make([]int32, a.Frames)
	}

	const fullScale = float64(math.MaxInt32)
	const toneAmp int32 = 1 << 30 // ~0.5 full scale
	noiseAmp := int32(math.Pow(10, -70.0/20) * fullScale)
	pauseAmp := int32(math.Pow(10, -50.0/20) * fullScale)

	frame := 0
	lfsr := uint32(0xACE1)
	next := func() float64 {
		bit := lfsr&1 ^ (lfsr>>1)&1 ^ (lfsr>>21)&1 ^ (lfsr>>31)&1
		lfsr = (lfsr >> 1) | (bit << 31)
		return 0.5 + 0.5*float64(lfsr)/float64(math.MaxUint32) // [0.5,1]
	}

	for _, p := range plan {
		end := frame + p.durMS*ms
		for i := frame; i < end; i++ {
			var s int32
			switch p.kind {
			case "tone":
				// cosine never lands on an exact zero at 440 Hz/48 kHz
				s = int32(float64(toneAmp) * math.Cos(2*math.Pi*440*float64(i)/testRate))
			case "digital":
				s = 0
			case "noise":
				sign := int32(1)
				if (i & 1) == 0 {
					sign = -1
				}
				s = sign * int32(float64(noiseAmp)*next())
			case "pause":
				sign := int32(1)
				if (i & 1) == 0 {
					sign = -1
				}
				s = sign * int32(float64(pauseAmp)*next())
			}
			a.Samples[0][i] = s
			a.Samples[1][i] = s
		}
		frame = end
	}
	return a
}

func findProgram(ivs []*Interval, level string, startFrame int) *Interval {
	for _, iv := range ivs {
		if iv.Channel == -1 && iv.Level == level && iv.Start == startFrame {
			return iv
		}
	}
	return nil
}

func TestDetectDigitalSilenceSampleAccurate(t *testing.T) {
	a := scenario(t, 300)
	ivs := Detect(a, DefaultConfig())

	ms := testRate / 1000
	d1 := findProgram(ivs, LevelDigital, 50*ms)
	if d1 == nil {
		t.Fatal("digital silence D1 not found at 50 ms")
	}
	if d1.End != 250*ms || d1.DurationMS != 200 {
		t.Fatalf("D1 boundary wrong: %+v", d1)
	}
	if d1.Verdict != VerdictRemovable || d1.Position != PosInterior {
		t.Fatalf("D1 classification wrong: %s / %s", d1.Verdict, d1.Position)
	}
	if !math.IsInf(float64(d1.PeakDB), -1) || d1.ZeroPct != 100 {
		t.Fatalf("D1 stats wrong: peak=%v zero=%.1f", d1.PeakDB, d1.ZeroPct)
	}

	tail := findProgram(ivs, LevelDigital, 2100*ms)
	if tail == nil || tail.Position != PosTrailing || tail.Verdict != VerdictBoundary {
		t.Fatalf("trailing silence wrong: %+v", tail)
	}

	digitals := 0
	for _, iv := range ivs {
		if iv.Channel == -1 && iv.Level == LevelDigital {
			digitals++
		}
	}
	if digitals != 3 {
		t.Fatalf("want 3 program-level digital intervals, got %d", digitals)
	}
}

func TestDetectNoiseFloorAndNaturalPause(t *testing.T) {
	a := scenario(t, 300)
	ivs := Detect(a, DefaultConfig())
	ms := testRate / 1000

	// The -70 dBFS block is a standalone noise-floor region at 450 ms.
	noise := findProgram(ivs, LevelNoise, 450*ms)
	if noise == nil {
		t.Fatal("noise-floor region not found")
	}
	if noise.End != 700*ms {
		t.Fatalf("noise region end frame = %d, want %d", noise.End, 700*ms)
	}
	if noise.PeakDB > -68 || noise.PeakDB < -72 {
		t.Fatalf("noise peak dBFS = %.1f, want ~ -70", noise.PeakDB)
	}
	if !strings.Contains(noise.Verdict, VerdictRemovable) {
		t.Fatalf("noise verdict = %q", noise.Verdict)
	}

	// The -50 dBFS soft gap is a pause-level acceptable natural pause.
	pause := findProgram(ivs, LevelPause, 900*ms)
	if pause == nil {
		t.Fatal("pause region not found")
	}
	if pause.DurationMS != 500 || pause.Verdict != VerdictAcceptable ||
		pause.Kind != "natural pause" {
		t.Fatalf("pause classification wrong: %+v", pause)
	}
	if pause.PeakDB <= -60 || pause.PeakDB >= -45 {
		t.Fatalf("pause peak = %.1f dBFS, want between -60 and -45", pause.PeakDB)
	}
}

func TestFiftyMillisecondDelta(t *testing.T) {
	a := scenario(t, 300)
	b := scenario(t, 250)
	ms := testRate / 1000

	dA := findProgram(Detect(a, DefaultConfig()), LevelDigital, 1600*ms)
	dB := findProgram(Detect(b, DefaultConfig()), LevelDigital, 1600*ms)
	if dA == nil || dB == nil {
		t.Fatal("second digital silence missing in one file")
	}
	delta := dA.DurationMS - dB.DurationMS
	if math.Abs(delta-50) > 0.01 {
		t.Fatalf("duration delta = %.3f ms, want 50", delta)
	}
}

func TestMinDurationFilter(t *testing.T) {
	base := DefaultConfig()
	a := scenario(t, 300)

	strict := base
	strict.MinDurMS = 500
	ivs := Detect(a, strict)
	for _, iv := range ivs {
		if iv.Channel == -1 && iv.Level == LevelDigital && iv.DurationMS < 500 {
			t.Fatalf("short interval leaked past filter: %+v", iv)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	bad := DefaultConfig()
	bad.NoiseDB = -40 // above pause-db -45
	if err := bad.Validate(); err == nil {
		t.Fatal("expected threshold-order error")
	}
	bad = DefaultConfig()
	bad.NaturalMS = bad.MicroMS - 1
	if err := bad.Validate(); err == nil {
		t.Fatal("expected duration-order error")
	}
}

func TestCLIEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	dir := t.TempDir()
	writeWAV := func(name string, d2MS int) string {
		a := scenario(t, d2MS)
		path := filepath.Join(dir, name)
		// downscale 32-bit samples to 16-bit for the on-disk fixture
		ch16 := make([][]int32, 2)
		for c := range ch16 {
			ch16[c] = make([]int32, a.Frames)
			for i, s := range a.Samples[c] {
				ch16[c][i] = s
			}
		}
		if err := os.WriteFile(path, buildWAV(t, testRate, 16, ch16), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	p1 := writeWAV("a.wav", 300)
	p2 := writeWAV("b.wav", 250)

	out, err := exec.Command("go", "run", ".", p1, p2).CombinedOutput()
	if err != nil {
		t.Fatalf("CLI run failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "cross-file comparison") ||
		!strings.Contains(text, "delta=") ||
		!strings.Contains(text, "digital") {
		t.Fatalf("CLI output missing comparison sections:\n%s", text)
	}

	jsonOut, err := exec.Command("go", "run", ".", "-json", p1).CombinedOutput()
	if err != nil {
		t.Fatalf("JSON run failed: %v\n%s", err, jsonOut)
	}
	var reports []fileReport
	if err := json.Unmarshal(jsonOut, &reports); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, jsonOut)
	}
	if len(reports) != 1 || len(reports[0].Intervals) == 0 {
		t.Fatalf("unexpected JSON payload: %s", jsonOut)
	}
}
