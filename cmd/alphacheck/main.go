package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"alphacheck/preview"
	"alphacheck/webm"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "inspect":
		err = runInspect(os.Args[2:])
	case "composite":
		err = runComposite(os.Args[2:])
	case "ivf":
		err = runIVF(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "alphacheck:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  alphacheck inspect [--json] [--all-frames] FILE
  alphacheck ivf FILE OUT-BASE
  alphacheck composite --color C.rgba --alpha A.gray --width W --height H --out PREFIX [--frame N]`)
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "write structured JSON")
	allFrames := fs.Bool("all-frames", false, "include every frame in JSON/text detail")
	maxFrames := fs.Int("max-frames", 24, "maximum frame records in JSON output; 0 omits them")
	previewMetrics := fs.String("preview-metrics", "", "optional composite metrics JSON to merge into verdict")
	requireVariation := fs.Bool("require-variation", false, "require alpha variation when merging preview metrics")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect requires exactly one WebM file")
	}
	opts := webm.DefaultOptions()
	if *allFrames {
		opts.MaxFrameDetails = -1
	} else {
		opts.MaxFrameDetails = *maxFrames
	}
	report, err := webm.InspectFile(fs.Arg(0), opts)
	if err != nil {
		return err
	}
	if *previewMetrics != "" {
		raw, err := os.ReadFile(*previewMetrics)
		if err != nil {
			return err
		}
		var result preview.Result
		if err := json.Unmarshal(raw, &result); err != nil {
			return err
		}
		if *requireVariation {
			result.Assessment = preview.Assess(result.Stats, true)
		}
		report.AttachPreview(result)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printTextReport(report)
	return nil
}
