package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"clickdetect/internal/click"
	"clickdetect/internal/wav"
)

func main() {
	opts := click.DefaultOptions()
	var asJSON bool
	flag.BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	flag.IntVar(&opts.WaveformBefore, "before", opts.WaveformBefore, "samples of waveform to print before each click")
	flag.IntVar(&opts.WaveformAfter, "after", opts.WaveformAfter, "samples of waveform to print after each click")
	flag.Float64Var(&opts.MinConfidence, "min-confidence", opts.MinConfidence, "minimum score in [0,1]")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] audio.wav\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(flag.CommandLine.Output(), "Detects splice clicks using sample discontinuity, phase jump, and low-frequency impulse evidence.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	audio, err := wav.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	result := click.Analyze(audio, opts)
	result.File = flag.Arg(0)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	printText(result, opts)
}

func printText(a click.Analysis, opts click.Options) {
	fmt.Printf("File: %s\n", a.File)
	fmt.Printf("Format: %d Hz, %d channel(s), %d frames (%.3f s)\n",
		a.SampleRate, a.Channels, a.Frames, a.DurationSec)
	fmt.Printf("Detected clicks: %d\n", len(a.Detections))
	for i, d := range a.Detections {
		channels := make([]string, len(d.Channels))
		for j, ch := range d.Channels {
			channels[j] = fmt.Sprintf("%d", ch)
		}
		fmt.Printf("\nClick %d: sample %d (%.6f s), channel(s) %s, confidence %.3f\n",
			i+1, d.Sample, d.TimeSec, strings.Join(channels, ","), d.Confidence)
		fmt.Printf("  indicators: %s\n", strings.Join(d.Flags, ", "))
		fmt.Printf("  jump=%.6g (%.2fx local p99, z=%.1f), phase=%.2f rad (coherence %.2f)\n",
			d.Features.JumpValue, d.Features.JumpRatio, d.Features.JumpZ,
			d.Features.PhaseJumpRad, d.Features.PhaseCoherence)
		fmt.Printf("  low-frequency step=%.6g (z=%.1f), impulse=%.6g (z=%.1f), burst samples=%d\n",
			d.Features.LowFreqStep, d.Features.LowFreqStepZ,
			d.Features.LowFreqImpulse, d.Features.LowFreqImpulseZ, d.Features.BurstSamples)
		printWaveform(d.Waveform)
	}
	if a.SuppressedTotal > 0 {
		fmt.Printf("\nNormal transient guard: suppressed %d sustained percussive event(s).\n", a.SuppressedTotal)
		limit := len(a.Suppressed)
		if limit > 5 {
			limit = 5
		}
		for _, s := range a.Suppressed[:limit] {
			fmt.Printf("  sample %d (%.6f s), channel(s) %v: %s\n", s.Sample, s.TimeSec, s.Channels, s.Reason)
		}
	}
}

func printWaveform(w click.Waveform) {
	if len(w.Samples) == 0 {
		return
	}
	fmt.Printf("  waveform samples %d..%d ('<' marks click sample):\n", w.FirstSample, w.FirstSample+len(w.Samples[0])-1)
	fmt.Print("    sample")
	for ch := range w.Samples {
		fmt.Printf("          ch%d", ch)
	}
	fmt.Println()
	for i := range w.Samples[0] {
		sample := w.FirstSample + i
		marker := " "
		if sample == w.ClickSample {
			marker = "<"
		}
		fmt.Printf("  %s %7d", marker, sample)
		for ch := range w.Samples {
			fmt.Printf(" %12.7f", w.Samples[ch][i])
		}
		fmt.Println()
	}
}
