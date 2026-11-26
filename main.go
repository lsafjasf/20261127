package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", "", "DCP root directory (defaults to CPL directory or current directory)")
	cplPath := flag.String("cpl", "", "path to a CPL XML file (optional if -root contains exactly one CPL)")
	asJSON := flag.Bool("json", false, "emit machine-readable JSON")
	flag.Parse()

	effectiveRoot := *root
	if effectiveRoot == "" && *cplPath == "" {
		effectiveRoot = "."
	}

	pkg, err := LoadPackage(effectiveRoot, *cplPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dcpcheck:", err)
		os.Exit(2)
	}

	report := Analyze(pkg)
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "dcpcheck: encode report:", err)
			os.Exit(2)
		}
	} else {
		fmt.Print(formatText(report))
	}

	if report.FirstUnplayable != nil || reportHasProblems(report) {
		os.Exit(1)
	}
}

func reportHasProblems(report *Report) bool {
	if len(report.Errors) > 0 || len(report.ReelDurationMismatches) > 0 {
		return true
	}
	for _, asset := range report.Assets {
		if asset.Status != "ok" || len(asset.Problems) > 0 {
			return true
		}
	}
	return false
}
