package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "monocheck:", err)
		os.Exit(1)
	}
}

func run() error {
	format := flag.String("format", "text", "output format: text or json")
	windowSize := flag.Int("window", 0, "FFT window size in samples; defaults to an automatic power of two")
	hopSize := flag.Int("hop", 0, "hop size in samples; defaults to half the window")
	flag.Parse()

	if flag.NArg() != 1 {
		return fmt.Errorf("usage: monocheck [--format text|json] [--window N] [--hop N] stereo.wav")
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("unsupported format %q", *format)
	}

	file, err := os.Open(flag.Arg(0))
	if err != nil {
		return err
	}
	defer file.Close()

	audio, err := ReadWAV(file)
	if err != nil {
		return err
	}
	report := analyze(audio.Left, audio.Right, audio.SampleRate, options{
		windowSize: *windowSize,
		hopSize:    *hopSize,
	})

	if *format == "json" {
		return writeJSON(report, os.Stdout)
	}
	return writeText(report, os.Stdout)
}
