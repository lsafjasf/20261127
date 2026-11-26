package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	cc "github.com/lsafjasf/20261127"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("colorcompare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("input", "-", "CSV file with id,converter_a,converter_b; - reads stdin")
	asJSON := flags.Bool("json", false, "emit JSON instead of a table")
	demo := flags.Bool("demo", false, "run the built-in dark/small-difference reversal example")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	var (
		samples []cc.Sample
		err     error
	)
	if *demo {
		samples = demoSamples()
	} else {
		samples, err = readSamplesFrom(*inputPath, stdin)
	}
	if err == nil {
		var analysis cc.Analysis
		analysis, err = cc.AnalyzeSamples(samples)
		if err == nil {
			if *asJSON {
				err = writeJSON(stdout, analysis)
			} else {
				writeTable(stdout, analysis)
			}
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "colorcompare: %v\n", err)
		return 1
	}
	return 0
}

func readSamplesFrom(path string, stdin io.Reader) ([]cc.Sample, error) {
	if path == "-" {
		return readSamplesReader(stdin, "<stdin>")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readSamplesReader(file, path)
}

func demoSamples() []cc.Sample {
	return []cc.Sample{
		{ID: "dark-small", A: cc.RGB24{R: 0x0E, G: 0x0C, B: 0x25}, B: cc.RGB24{R: 0x0E, G: 0x0C, B: 0x26}},
		{ID: "light-small", A: cc.RGB24{R: 0xDE, G: 0xD3, B: 0x76}, B: cc.RGB24{R: 0xDF, G: 0xD3, B: 0x78}},
		{ID: "near-gray", A: cc.RGB24{R: 128, G: 128, B: 128}, B: cc.RGB24{R: 128, G: 128, B: 129}},
		{ID: "high-sat", A: cc.RGB24{R: 255, G: 0, B: 0}, B: cc.RGB24{R: 0, G: 255, B: 0}},
	}
}
