package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"hdr-sdr-manifest/internal/manifest"
)

var version = "0.1.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("hdr-sdr-manifest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "delivery input JSON (required)")
	outputPath := flags.String("out", "-", "manifest output path, or - for stdout")
	format := flags.String("format", "json", "output format: json or markdown")
	ffprobePath := flags.String("ffprobe", envOrDefault("FFPROBE", "ffprobe"), "ffprobe executable")
	ffmpegPath := flags.String("ffmpeg", envOrDefault("FFMPEG", "ffmpeg"), "ffmpeg executable")
	allowIncomplete := flags.Bool("allow-incomplete", false, "return exit code 0 even when the manifest is incomplete")
	showVersion := flags.Bool("version", false, "print version and exit")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: hdr-sdr-manifest -config delivery.json [-out manifest.json] [-format json|markdown]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "hdr-sdr-manifest %s\n", version)
		return 0
	}
	if strings.TrimSpace(*configPath) == "" {
		flags.Usage()
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected positional arguments: %s\n", strings.Join(flags.Args(), " "))
		return 2
	}
	formatName := strings.ToLower(strings.TrimSpace(*format))
	if formatName != "json" && formatName != "markdown" {
		fmt.Fprintf(stderr, "unsupported format %q; use json or markdown\n", *format)
		return 2
	}

	config, err := manifest.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "invalid delivery input: %v\n", err)
		return 2
	}
	prober := manifest.NewExecProbe(*ffprobePath, *ffmpegPath)
	report, err := manifest.Assess(ctx, config, manifest.AssessOptions{
		Prober:      prober,
		ToolVersion: version,
	})
	if err != nil {
		fmt.Fprintf(stderr, "assessment failed: %v\n", err)
		return 2
	}

	var output []byte
	if formatName == "markdown" {
		output = manifest.RenderMarkdown(report)
	} else {
		output, err = manifest.MarshalJSON(report)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 2
		}
	}
	if err := writeOutput(*outputPath, output, stdout); err != nil {
		fmt.Fprintf(stderr, "write manifest: %v\n", err)
		return 2
	}

	destination := *outputPath
	if destination == "-" {
		destination = "stdout"
	}
	fmt.Fprintf(stderr, "manifest %s: status=%s checks=%d passed=%d failed=%d\n",
		destination,
		report.Status,
		report.Summary.TotalChecks,
		report.Summary.PassedChecks,
		report.Summary.FailedChecks,
	)
	if !report.Complete && !*allowIncomplete {
		return 1
	}
	return 0
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func writeOutput(path string, data []byte, stdout io.Writer) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	directory := filepath.Dir(path)
	if directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	temp, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() {
		temp.Close()
		os.Remove(tempName)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		os.Remove(tempName)
		return err
	}
	return nil
}
