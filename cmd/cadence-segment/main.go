package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lsafjasf/20261127/cadence"
	"github.com/lsafjasf/20261127/media"
)

type cliConfig struct {
	input       string
	format      string
	pretty      bool
	window      int
	minSegment  int
	change      float64
	cut         float64
	fieldWidth  int
	fieldHeight int
	ffmpeg      string
	ffprobe     string
	timeout     time.Duration
	failUnknown bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if cfg.timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, cfg.timeout)
		defer timeoutCancel()
	}
	decoded, err := media.Extract(ctx, cfg.input, media.Options{
		FFmpeg:      cfg.ffmpeg,
		FFprobe:     cfg.ffprobe,
		FieldWidth:  cfg.fieldWidth,
		FieldHeight: cfg.fieldHeight,
	})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	report, err := cadence.Analyze(decoded, cadence.Options{
		WindowFrames:      cfg.window,
		MinSegmentFrames:  cfg.minSegment,
		ChangePenalty:     cfg.change,
		SceneCutThreshold: cfg.cut,
	})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if cfg.format == "text" {
		writeText(stdout, cfg.input, report)
	} else {
		enc := json.NewEncoder(stdout)
		if cfg.pretty {
			enc.SetIndent("", "  ")
		}
		enc.SetEscapeHTML(false)
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
	}
	if cfg.failUnknown {
		for _, segment := range report.Segments {
			if segment.Cadence == cadence.CadenceUnknown {
				return 3
			}
		}
	}
	return 0
}

func parseFlags(args []string, stderr *os.File) (cliConfig, error) {
	cfg := cliConfig{
		format:      "json",
		pretty:      true,
		window:      16,
		minSegment:  4,
		change:      0.32,
		cut:         0.24,
		fieldWidth:  32,
		fieldHeight: 18,
		ffmpeg:      "ffmpeg",
		ffprobe:     "ffprobe",
		timeout:     30 * time.Minute,
	}
	fs := flag.NewFlagSet("cadence-segment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.input, "input", "", "input video path")
	fs.StringVar(&cfg.input, "i", "", "input video path")
	fs.StringVar(&cfg.format, "format", cfg.format, "output format: json or text")
	fs.BoolVar(&cfg.pretty, "pretty", cfg.pretty, "indent JSON output")
	fs.IntVar(&cfg.window, "window", cfg.window, "local evidence window in coded frames")
	fs.IntVar(&cfg.minSegment, "min-segment", cfg.minSegment, "minimum segment length in frames")
	fs.Float64Var(&cfg.change, "change-penalty", cfg.change, "cadence-change cost")
	fs.Float64Var(&cfg.cut, "scene-cut", cfg.cut, "normalized scene-cut threshold")
	fs.IntVar(&cfg.fieldWidth, "field-width", cfg.fieldWidth, "analysis field width")
	fs.IntVar(&cfg.fieldHeight, "field-height", cfg.fieldHeight, "analysis field height")
	fs.StringVar(&cfg.ffmpeg, "ffmpeg", cfg.ffmpeg, "ffmpeg executable")
	fs.StringVar(&cfg.ffprobe, "ffprobe", cfg.ffprobe, "ffprobe executable")
	fs.DurationVar(&cfg.timeout, "timeout", cfg.timeout, "decode timeout (0 disables)")
	fs.BoolVar(&cfg.failUnknown, "fail-on-unknown", false, "exit 3 if any segment is unknown")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: cadence-segment -i INPUT [options]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.input == "" && fs.NArg() == 1 {
		cfg.input = fs.Arg(0)
	}
	if cfg.input == "" {
		fs.Usage()
		return cfg, errors.New("missing input video")
	}
	cfg.format = strings.ToLower(cfg.format)
	if cfg.format != "json" && cfg.format != "text" {
		return cfg, fmt.Errorf("unsupported format %q", cfg.format)
	}
	if cfg.fieldWidth < 8 || cfg.fieldHeight < 8 {
		return cfg, errors.New("field dimensions must be at least 8x8")
	}
	return cfg, nil
}

func writeText(w *os.File, input string, report cadence.Report) {
	fmt.Fprintf(w, "input: %s\n", input)
	fmt.Fprintf(w, "frames: %d  duration: %.3fs  field rate: %.3f\n",
		report.FrameCount, report.DurationSec, report.FieldRate)
	for _, segment := range report.Segments {
		fmt.Fprintf(w, "%d %8.3f-%8.3fs  %-8s %-3s conf=%.2f  %s\n",
			segment.Index, segment.StartSec, segment.EndSec,
			segment.Cadence, segment.FieldOrder, segment.Confidence, segment.Recommended)
		for _, basis := range segment.Evidence.Basis {
			fmt.Fprintf(w, "    - %s\n", basis)
		}
	}
	for _, warning := range report.Warnings {
		fmt.Fprintln(w, "warning:", warning)
	}
}
