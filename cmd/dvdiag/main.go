package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/lsafjasf/20261127/internal/diag"
)

func main() {
	jsonOut := flag.Bool("json", false, "write the complete report as JSON")
	capsPath := flag.String("caps", "", "device capability JSON file")
	scanSamples := flag.Int("scan-samples", 0, "video samples to scan per segment; 0 scans all")
	maxScanMiB := flag.Int64("max-scan-mib", 64, "maximum sample bytes read per segment")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: dvdiag [flags] media-file...\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	var devices []diag.DeviceCapabilities
	var err error
	if *capsPath != "" {
		devices, err = diag.LoadDevices(*capsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load capabilities: %v\n", err)
			os.Exit(2)
		}
	}
	opts := diag.Options{
		ScanSamples:  *scanSamples,
		MaxScanBytes: *maxScanMiB << 20,
	}
	report := diag.Report{}
	failures := 0
	for _, path := range flag.Args() {
		file := diag.AnalyzeFile(path, devices, opts)
		if file.Error != "" {
			failures++
		}
		report.Files = append(report.Files, file)
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			os.Exit(2)
		}
	} else {
		writeText(report)
	}
	if failures == len(report.Files) {
		os.Exit(1)
	}
}
