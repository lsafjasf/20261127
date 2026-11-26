package lighting

import (
	"math"
	"testing"
)

func analyzeScene(t *testing.T, frames []Frame) Report {
	t.Helper()
	cfg := DefaultConfig()
	cfg.MaxAnalysisWidth = 96
	analyzer, err := NewAnalyzer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, frame := range frames {
		if err := analyzer.AddFrame(frame); err != nil {
			t.Fatal(err)
		}
	}
	report, err := analyzer.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func render(w, h int, fps, duration float64, fn func(t float64, x, y int) float64) []Frame {
	n := int(math.Round(fps * duration))
	frames := make([]Frame, 0, n)
	for i := 0; i < n; i++ {
		tm := float64(i) / fps
		luma := make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := fn(tm, x, y)
				luma[y*w+x] = byte(clamp(math.Round(v), 0, 255))
			}
		}
		frames = append(frames, Frame{
			Luma: luma, Width: w, Height: h, Timestamp: tm,
		})
	}
	return frames
}

func baseTexture(x, y int) float64 {
	return 88 + float64((x*13+y*7)%24)
}

func TestLEDFlickerIsSeparateFromForegroundMotion(t *testing.T) {
	w, h, fps, duration := 64, 48, 30.0, 6.0
	frames := render(w, h, fps, duration, func(tm float64, x, y int) float64 {
		v := baseTexture(x, y) + 9*math.Sin(2*math.Pi*10*tm)
		x0 := 4 + int(tm*12)%(w-18)
		if x >= x0 && x < x0+12 && y >= 16 && y < 32 {
			v = 198 + 7*math.Sin(2*math.Pi*10*tm)
		}
		return v
	})
	report := analyzeScene(t, frames)
	t.Logf("classification=%+v flicker=%+v motion=%+v background=%+v events=%+v",
		report.Classification, report.Evidence.Flicker,
		report.Evidence.ForegroundMotion, report.Evidence.StableBackground,
		report.Events)
	minI, maxI := 0, 0
	for i, v := range report.Curves.StableBackgroundLuma {
		if v < report.Curves.StableBackgroundLuma[minI] {
			minI = i
		}
		if v > report.Curves.StableBackgroundLuma[maxI] {
			maxI = i
		}
	}
	t.Logf("background min[%d]=%.2f max[%d]=%.2f stable=%.2f/%.2f",
		minI, report.Curves.StableBackgroundLuma[minI], maxI,
		report.Curves.StableBackgroundLuma[maxI],
		report.Curves.StableBackgroundFraction[minI],
		report.Curves.StableBackgroundFraction[maxI])
	if report.Classification.Primary != "led_flicker" {
		t.Fatalf("primary %q, want led_flicker", report.Classification.Primary)
	}
	if !report.Evidence.Flicker.Periodic {
		t.Fatal("periodic flicker was not detected")
	}
	if report.Evidence.ForegroundMotion.MeanFraction <= 0.01 {
		t.Fatal("moving foreground was not detected")
	}
	if report.Evidence.StableBackground.MeanFraction <= 0.4 {
		t.Fatalf("stable background fraction %.3f is too low",
			report.Evidence.StableBackground.MeanFraction)
	}
	if math.Abs(report.Evidence.Flicker.PeakHz-10) > 0.3 {
		t.Fatalf("flicker peak %.3f Hz, want 10 Hz", report.Evidence.Flicker.PeakHz)
	}
}

func TestAutoExposureStepIsClassifiedAndRetained(t *testing.T) {
	w, h, fps, duration := 96, 72, 30.0, 5.0
	frames := render(w, h, fps, duration, func(tm float64, x, y int) float64 {
		v := baseTexture(x, y)
		if tm >= 2.0 {
			v *= 0.70
		}
		return v
	})
	report := analyzeScene(t, frames)
	t.Logf("classification=%+v events=%+v", report.Classification, report.Events)
	if report.Classification.Primary != "auto_exposure" {
		t.Fatalf("primary %q, want auto_exposure", report.Classification.Primary)
	}
	event := findEvent(report.Events, "auto_exposure")
	if event == nil {
		t.Fatal("auto exposure event missing")
	}
	if !event.Retained {
		t.Fatal("auto exposure event was not retained")
	}
	if event.Amplitude > -15 {
		t.Fatalf("event amplitude %.2f, want a clear negative step", event.Amplitude)
	}
}

func TestCloudShadowSlowChangeIsClassifiedAndRetained(t *testing.T) {
	w, h, fps, duration := 96, 72, 30.0, 8.0
	frames := render(w, h, fps, duration, func(tm float64, x, y int) float64 {
		boundary := -0.15 + 1.30*tm/duration
		xn := float64(x) / float64(w-1)
		shadow := 14 / (1 + math.Exp(-9*(boundary-xn)))
		return baseTexture(x, y) - shadow
	})
	report := analyzeScene(t, frames)
	t.Logf("classification=%+v events=%+v", report.Classification, report.Events)
	if report.Classification.Primary != "cloud_shadow" {
		t.Fatalf("primary %q, want cloud_shadow", report.Classification.Primary)
	}
	event := findEvent(report.Events, "cloud_shadow")
	if event == nil {
		t.Fatal("cloud shadow event missing")
	}
	if !event.Retained || report.Evidence.GlobalBrightness.SustainedRange < 8 {
		t.Fatalf("sustained change was not retained: event=%+v range=%.2f",
			event, report.Evidence.GlobalBrightness.SustainedRange)
	}
}

func TestFlickerDoesNotEraseSustainedCloudChange(t *testing.T) {
	w, h, fps, duration := 96, 72, 30.0, 8.0
	frames := render(w, h, fps, duration, func(tm float64, x, y int) float64 {
		boundary := -0.15 + 1.30*tm/duration
		xn := float64(x) / float64(w-1)
		shadow := 12 / (1 + math.Exp(-9*(boundary-xn)))
		return baseTexture(x, y) - shadow + 8*math.Sin(2*math.Pi*10*tm)
	})
	report := analyzeScene(t, frames)
	t.Logf("classification=%+v flicker=%+v events=%+v",
		report.Classification, report.Evidence.Flicker, report.Events)
	if report.Classification.Primary != "mixed" {
		t.Fatalf("primary %q, want mixed", report.Classification.Primary)
	}
	if !report.Evidence.Flicker.Periodic {
		t.Fatal("periodic flicker was lost after combining with cloud shadow")
	}
	if findEvent(report.Events, "cloud_shadow") == nil {
		t.Fatal("sustained cloud event was erased")
	}
	if report.Evidence.GlobalBrightness.SustainedRange < 7 {
		t.Fatalf("sustained range %.2f is too small", report.Evidence.GlobalBrightness.SustainedRange)
	}
	curves := report.Curves.SustainedLuma
	if curves[len(curves)-1]-curves[0] > -7 {
		t.Fatalf("sustained curve did not preserve darkening: %.2f",
			curves[len(curves)-1]-curves[0])
	}
}

func TestFlickerAndAutoExposureAreBothReported(t *testing.T) {
	w, h, fps, duration := 96, 72, 30.0, 5.0
	frames := render(w, h, fps, duration, func(tm float64, x, y int) float64 {
		v := baseTexture(x, y) + 9*math.Sin(2*math.Pi*10*tm)
		if tm >= 2.0 {
			v -= 20
		}
		return v
	})
	report := analyzeScene(t, frames)
	t.Logf("classification=%+v flicker=%+v events=%+v",
		report.Classification, report.Evidence.Flicker, report.Events)
	if report.Classification.Primary != "mixed" {
		t.Fatalf("primary %q, want mixed", report.Classification.Primary)
	}
	if !report.Evidence.Flicker.Periodic || !report.Evidence.Flicker.AutoExposureConfound {
		t.Fatalf("expected both flicker and exposure evidence: %+v",
			report.Evidence.Flicker)
	}
	if findEvent(report.Events, "auto_exposure") == nil {
		t.Fatalf("auto exposure event missing: %+v", report.Events)
	}
}

func TestLargeForegroundMotionIsNotCloudShadow(t *testing.T) {
	w, h, fps, duration := 96, 72, 30.0, 6.0
	frames := render(w, h, fps, duration, func(tm float64, x, y int) float64 {
		v := baseTexture(x, y)
		x0 := int(tm*18) % (w + 32)
		if x >= x0-32 && x < x0 && y >= 8 && y < h-8 {
			v = 205
		}
		return v
	})
	report := analyzeScene(t, frames)
	t.Logf("classification=%+v background=%+v motion=%+v",
		report.Classification, report.Evidence.StableBackground,
		report.Evidence.ForegroundMotion)
	if report.Evidence.ForegroundMotion.MeanFraction < 0.10 {
		t.Fatalf("foreground motion %.3f was not detected",
			report.Evidence.ForegroundMotion.MeanFraction)
	}
	if report.Classification.Primary != "stable" {
		t.Fatalf("primary %q, want stable; events=%+v",
			report.Classification.Primary, report.Events)
	}
	if report.Evidence.GlobalBrightness.Range < 8 {
		t.Fatalf("global range %.2f should show foreground contamination",
			report.Evidence.GlobalBrightness.Range)
	}
}

func TestStaticSceneIsStable(t *testing.T) {
	w, h, fps, duration := 64, 48, 30.0, 3.0
	frames := render(w, h, fps, duration, func(_ float64, x, y int) float64 {
		return baseTexture(x, y)
	})
	report := analyzeScene(t, frames)
	if report.Classification.Primary != "stable" {
		t.Fatalf("primary %q, want stable; events=%+v",
			report.Classification.Primary, report.Events)
	}
	if len(report.Events) != 0 {
		t.Fatalf("static scene produced events: %+v", report.Events)
	}
}

func TestSingleFrameProducesUsableReport(t *testing.T) {
	analyzer, err := NewAnalyzer(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	frame := Frame{
		Luma:      make([]byte, 32*24),
		Width:     32,
		Height:    24,
		Timestamp: 0,
	}
	if err := analyzer.AddFrame(frame); err != nil {
		t.Fatal(err)
	}
	report, err := analyzer.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if report.Source.FrameCount != 1 || len(report.Curves.TimeS) != 1 {
		t.Fatalf("unexpected source %+v", report.Source)
	}
	if len(report.Bands.Global) != 3 || len(report.Events) != 0 {
		t.Fatalf("unexpected one-frame report: bands=%v events=%v",
			report.Bands, report.Events)
	}
}

func TestAnalyzerRejectsNonMonotonicTimestamps(t *testing.T) {
	analyzer, err := NewAnalyzer(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	frame := Frame{Luma: make([]byte, 16*16), Width: 16, Height: 16}
	if err := analyzer.AddFrame(frame); err != nil {
		t.Fatal(err)
	}
	frame.Timestamp = 0
	if err := analyzer.AddFrame(frame); err == nil {
		t.Fatal("non-monotonic timestamp was accepted")
	}
}

func findEvent(events []Event, kind string) *Event {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}
