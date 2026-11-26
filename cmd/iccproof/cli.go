package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	iccproof "iccproof"
)

func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

// Run executes the CLI and returns a process-style exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "proof":
		return runProof(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "demo":
		return runDemo(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `iccproof — explicit ICC soft-proofing and boundary reports

Usage:
  iccproof proof --photo photo.jpg --source source.icc --media paper.icc \
      --intent perceptual,relative-colorimetric,absolute-colorimetric \
      --format json --out report.json
  iccproof proof --input request.json --format csv --out samples.csv
  iccproof inspect --profile profile.icc [--json]
  iccproof demo [--format json|csv|text]

Intents:
  perceptual
  relative-colorimetric
  absolute-colorimetric
  saturation (accepted, not part of the default three-way comparison)

Profiles are never replaced by a built-in sRGB profile. If --source/--media is
absent and the request JSON does not provide it, the command fails.
`)
}

type proofFlags struct {
	input       string
	photo       string
	source      string
	media       string
	display     string
	intent      string
	format      string
	out         string
	stride      int
	maxSamples  int
	allPixels   bool
	bits        int
	failOnError bool
}

func runProof(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("proof", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f proofFlags
	fs.StringVar(&f.input, "input", "", "JSON request path (or - for stdin)")
	fs.StringVar(&f.photo, "photo", "", "PNG/JPEG photo to sample")
	fs.StringVar(&f.source, "source", "", "explicit source ICC/.icm profile path")
	fs.StringVar(&f.media, "media", "", "explicit media/proof ICC/.icm profile path")
	fs.StringVar(&f.media, "proof", "", "alias for --media")
	fs.StringVar(&f.display, "display", "", "optional explicit display ICC for soft-proof preview")
	fs.StringVar(&f.intent, "intent", "all", "comma-separated intents or 'all'")
	fs.StringVar(&f.format, "format", "", "json, csv, or text (default: infer from --out or text)")
	fs.StringVar(&f.out, "out", "", "output file (default stdout)")
	fs.IntVar(&f.stride, "stride", 0, "pixel stride for photo sampling")
	fs.IntVar(&f.maxSamples, "max-samples", 256, "maximum photo samples")
	fs.BoolVar(&f.allPixels, "all-pixels", false, "sample every pixel (can produce a very large report)")
	fs.IntVar(&f.bits, "bits", 8, "output quantization bits: 8 or 16")
	fs.BoolVar(&f.failOnError, "fail-on-sample-error", true, "exit 1 if any sample could not be transformed")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	req, err := buildProofRequest(f, stdinReader{})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	report, err := iccproof.Proof(req)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitCodeForProofError(err)
	}

	format := chooseFormat(f.format, f.out)
	var writer io.Writer = stdout
	var closer io.Closer
	if f.out != "" {
		file, err := os.Create(f.out)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
		writer = file
		closer = file
	}
	if err := writeReport(report, format, writer); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		if closer != nil {
			_ = closer.Close()
		}
		return 1
	}
	if closer != nil {
		if err := closer.Close(); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(stderr, "wrote %s report to %s\n", format, f.out)
	}
	if f.failOnError && reportHasSampleError(report) {
		return 1
	}
	return 0
}

type stdinReader struct{}

func (stdinReader) Read(p []byte) (int, error) { return os.Stdin.Read(p) }

func buildProofRequest(f proofFlags, input io.Reader) (iccproof.Request, error) {
	var req iccproof.Request
	if f.input != "" {
		var reader io.Reader
		if f.input == "-" {
			reader = input
		} else {
			file, err := os.Open(f.input)
			if err != nil {
				return req, err
			}
			defer file.Close()
			reader = file
		}
		var err error
		req, err = iccproof.ParseRequestJSON(reader)
		if err != nil {
			return req, err
		}
	}

	if f.photo != "" {
		req.PhotoPath = f.photo
		// An explicit --photo replaces sample arrays inherited from JSON.
		req.Samples = nil
	}
	if f.source != "" {
		req.Source = iccproof.ProfileRef{Path: f.source}
	}
	if f.media != "" {
		req.Media = iccproof.ProfileRef{Path: f.media}
	}
	if f.display != "" {
		req.Display = &iccproof.ProfileRef{Path: f.display}
	}
	intents, err := iccproof.ParseIntents(f.intent)
	if err != nil {
		return req, err
	}
	req.Intents = intents
	if f.stride > 0 || f.maxSamples != 256 || f.allPixels {
		grid := &iccproof.SampleGrid{Stride: f.stride, MaxSamples: f.maxSamples}
		if f.allPixels {
			grid.Stride = 1
			grid.MaxSamples = -1
		}
		req.SampleGrid = grid
	}
	if f.bits != 0 {
		req.Config.OutputBitDepth = f.bits
	}
	return req, nil
}

func chooseFormat(format, out string) string {
	if format != "" {
		return strings.ToLower(format)
	}
	switch strings.ToLower(filepath.Ext(out)) {
	case ".json":
		return "json"
	case ".csv":
		return "csv"
	case ".txt", ".text":
		return "text"
	default:
		return "text"
	}
}

func writeReport(report *iccproof.Report, format string, w io.Writer) error {
	switch format {
	case "json":
		return iccproof.WriteJSON(report, w)
	case "csv":
		return iccproof.WriteCSV(report, w)
	case "text", "":
		return iccproof.WriteText(report, w)
	default:
		return fmt.Errorf("unsupported format %q (want json, csv, or text)", format)
	}
}

func reportHasSampleError(r *iccproof.Report) bool {
	for _, result := range r.Results {
		if result.Error != "" {
			return true
		}
	}
	return false
}

func exitCodeForProofError(err error) int {
	if errors.Is(err, iccproof.ErrProfileRequired) || errors.Is(err, iccproof.ErrProfileNotFound) {
		return 3
	}
	return 2
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var profilePath string
	var asJSON bool
	fs.StringVar(&profilePath, "profile", "", "ICC/.icm profile path")
	fs.BoolVar(&asJSON, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	loaded, err := iccproof.LoadProfile(iccproof.RoleSource, iccproof.ProfileRef{Path: profilePath})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitCodeForProofError(err)
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(loaded.Info)
		return 0
	}
	fmt.Fprintf(stdout, "%s\n  version: %s\n  class: %s\n  color: %s\n  PCS: %s\n  tags(%d): %s\n  sha256: %s\n",
		loaded.Info.Path, loaded.Info.Version, loaded.Info.DeviceClass,
		loaded.Info.ColorSpace, loaded.Info.PCS, loaded.Info.TagCount,
		strings.Join(loaded.Info.Tags, ", "), loaded.Info.SHA256)
	return 0
}
