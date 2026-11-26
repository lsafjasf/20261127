package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	tearcheck "tearcheck"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "tearcheck:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		printUsage(os.Stderr)
		return errors.New("missing command")
	}
	switch arguments[0] {
	case "signals", "json":
		return runSignals(arguments[1:])
	case "video", "analyze":
		return runVideo(arguments[1:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func runSignals(arguments []string) error {
	flags := flag.NewFlagSet("signals", flag.ContinueOnError)
	inputPath := flags.String("input", "", "JSON request path; default reads stdin")
	outputPath := flags.String("output", "", "report path; default writes stdout")
	format := flags.String("format", "json", "output format: json|text")
	if err := flags.Parse(arguments); err != nil {
		return err
	}

	var reader io.Reader = os.Stdin
	if *inputPath != "" {
		file, err := os.Open(*inputPath)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	} else if stat, err := os.Stdin.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		printUsage(os.Stderr)
		return errors.New("signals requires --input or JSON on stdin")
	}
	request, err := tearcheck.DecodeRequest(reader)
	if err != nil {
		return err
	}
	report := tearcheck.AnalyzeRequest(request)
	return writeReport(report, *outputPath, *format)
}

func runVideo(arguments []string) error {
	flags := flag.NewFlagSet("video", flag.ContinueOnError)
	metadataPath := flags.String("metadata", "", "optional capture/codec/content metadata JSON")
	outputPath := flags.String("output", "", "report path; default writes stdout")
	format := flags.String("format", "json", "output format: json|text")
	maxFrames := flags.Int("frames", 0, "maximum frames to decode; 0 means all")
	ffmpegBin := flags.String("ffmpeg", envOr("FFMPEG_BIN", "ffmpeg"), "ffmpeg binary")
	ffprobeBin := flags.String("ffprobe", envOr("FFPROBE_BIN", "ffprobe"), "ffprobe binary")
	positionals, err := parseInterspersed(flags, arguments)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("video command requires exactly one video path")
	}
	videoPath := positionals[0]

	metadata := tearcheck.Metadata{}
	if *metadataPath != "" {
		var err error
		metadata, err = tearcheck.ReadMetadataFile(*metadataPath)
		if err != nil {
			return err
		}
	}
	report, err := tearcheck.AnalyzeVideo(context.Background(), videoPath, tearcheck.VideoOptions{
		FFmpegBin:  *ffmpegBin,
		FFprobeBin: *ffprobeBin,
		MaxFrames:  *maxFrames,
		Metadata:   metadata,
		OnWarning:  func(message string) { fmt.Fprintln(os.Stderr, "warning:", message) },
	})
	if err != nil {
		return err
	}
	return writeReport(report, *outputPath, *format)
}

func parseInterspersed(flags *flag.FlagSet, arguments []string) ([]string, error) {
	var positionals []string
	remaining := arguments
	for len(remaining) > 0 {
		if err := flags.Parse(remaining); err != nil {
			return nil, err
		}
		args := flags.Args()
		if len(args) == 0 {
			break
		}
		positionals = append(positionals, args[0])
		remaining = args[1:]
	}
	return positionals, nil
}

func writeReport(report *tearcheck.Report, outputPath, format string) error {
	var writer io.Writer = os.Stdout
	if outputPath != "" {
		file, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	switch format {
	case "json":
		return tearcheck.WriteReportJSON(writer, report)
	case "text":
		_, err := io.WriteString(writer, tearcheck.FormatTextReport(report))
		return err
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `tearcheck - distinguish capture sync, encoding, and intentional game tearing

Usage:
  tearcheck signals [--input request.json] [--output report.json] [--format text]
  tearcheck video <media-file> [--metadata sidecar.json] [--frames N] [--format text]

The signals command accepts normalized observations. The video command requires
ffmpeg and ffprobe on PATH (or FFMPEG_BIN/FFPROBE_BIN). Low-confidence events
are always copied to manual_reviews.`)
}
