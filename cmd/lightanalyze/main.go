package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"lightanalyze/lighting"
	"lightanalyze/y4m"
)

func main() {
	cfg := lighting.DefaultConfig()
	var (
		input      string
		output     string
		csvPath    string
		pretty     bool
		flickerLow float64
	)
	flag.StringVar(&input, "in", "-", "input Y4M path, or - for stdin")
	flag.StringVar(&output, "out", "-", "output JSON path, or - for stdout")
	flag.StringVar(&csvPath, "csv", "", "optional per-frame CSV path")
	flag.BoolVar(&pretty, "pretty", true, "indent JSON output")
	flag.IntVar(&cfg.MaxAnalysisWidth, "max-width", cfg.MaxAnalysisWidth,
		"downsample analysis to at most this width")
	flag.Float64Var(&cfg.MotionThreshold, "motion-threshold", cfg.MotionThreshold,
		"normalized block residual treated as motion")
	flag.Float64Var(&cfg.EventSlopeThreshold, "event-slope", cfg.EventSlopeThreshold,
		"sustained-change slope threshold in luma/second")
	flag.Float64Var(&cfg.EventMinAmplitude, "event-min-amplitude", cfg.EventMinAmplitude,
		"minimum retained light-change amplitude in luma")
	flag.Float64Var(&cfg.SlowCutoffHz, "slow-cutoff", cfg.SlowCutoffHz,
		"upper edge of the sustained light band")
	flag.Float64Var(&flickerLow, "flicker-low", cfg.FlickerBandLowHz,
		"lower edge of the flicker band")
	flag.Parse()

	if flag.NArg() > 0 {
		input = flag.Arg(0)
	}
	cfg.FlickerBandLowHz = flickerLow
	if cfg.FlickerBandLowHz < cfg.ExposureBandHighHz {
		cfg.ExposureBandHighHz = cfg.FlickerBandLowHz
	}

	in, closeIn, err := openInput(input)
	if err != nil {
		fail(err)
	}
	defer closeIn()

	reader, err := y4m.NewReader(in)
	if err != nil {
		fail(err)
	}
	analyzer, err := lighting.NewAnalyzer(cfg)
	if err != nil {
		fail(err)
	}
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fail(err)
		}
		if err := analyzer.AddFrame(lighting.Frame{
			Luma:      frame.Luma,
			Width:     frame.Width,
			Height:    frame.Height,
			Timestamp: frame.Timestamp,
		}); err != nil {
			fail(err)
		}
	}
	report, err := analyzer.Finish()
	if err != nil {
		fail(err)
	}
	if err := writeJSON(output, report, pretty); err != nil {
		fail(err)
	}
	if csvPath != "" {
		if err := writeCSV(csvPath, report.Curves); err != nil {
			fail(err)
		}
	}
}

func openInput(path string) (io.Reader, func(), error) {
	if path == "-" {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func writeJSON(path string, report lighting.Report, pretty bool) error {
	var out io.Writer = os.Stdout
	closeOut := func() {}
	if path != "-" {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		out = f
		closeOut = func() { _ = f.Close() }
	}
	defer closeOut()
	enc := json.NewEncoder(out)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lightanalyze:", err)
	os.Exit(1)
}
