package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	iccproof "iccproof"
)

func runDemo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "json, csv, or text")
	out := fs.String("out", "", "output file (default stdout)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	source := iccproof.ProfileRef{Descriptor: &iccproof.ColorProfile{
		ID: "demo-wide-gamut-source", Description: "Explicit BT.2020-like demo profile, not an implicit default",
		ColorSpace: "RGB", Gamma: 2.2, White: iccproof.D65XYY(),
		Red:   iccproof.XYY{X: 0.708, Y: 0.292},
		Green: iccproof.XYY{X: 0.170, Y: 0.797},
		Blue:  iccproof.XYY{X: 0.131, Y: 0.046},
	}}
	media := iccproof.ProfileRef{Descriptor: &iccproof.ColorProfile{
		ID: "demo-coated-paper", Description: "Explicit narrower RGB paper surrogate with D50 white",
		ColorSpace: "RGB", Gamma: 2.4, White: iccproof.D50XYY(),
		Red:   iccproof.XYY{X: 0.640, Y: 0.330},
		Green: iccproof.XYY{X: 0.300, Y: 0.600},
		Blue:  iccproof.XYY{X: 0.150, Y: 0.060},
	}}
	req := iccproof.Request{
		Source: source, Media: media, Intents: iccproof.AllProofIntents(),
		Config: iccproof.ProofConfig{OutputBitDepth: 8},
		Samples: []iccproof.Sample{
			{ID: "wide-green", RGB: []float64{0, 1, 0}},
			{ID: "bright-cyan", RGB: []float64{0, 0.9, 1}},
			{ID: "print-yellow", RGB: []float64{1, 0.86, 0}},
			{ID: "magenta-edge", RGB: []float64{1, 0, 0.75}},
			{ID: "skin", RGB: []float64{0.72, 0.49, 0.36}},
			{ID: "neutral-50", RGB: []float64{0.5, 0.5, 0.5}},
			{ID: "source-overrange", RGB: []float64{0, 1.12, 0}},
		},
	}
	report, err := iccproof.Proof(req)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	writer := stdout
	var file *os.File
	if *out != "" {
		file, err = os.Create(*out)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
		defer file.Close()
		writer = file
	}
	if err := writeReport(report, *format, writer); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if file != nil {
		fmt.Fprintf(stderr, "wrote demo %s report to %s\n", *format, *out)
	}
	return 0
}
