package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"streamcheck/internal/check"
)

func main() {
	format := flag.String("format", "text", "output format: text or json")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s evidence.json [-format text|json]\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "pass - as the input path to read JSON from stdin")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	bundle, err := readBundle(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	report, err := check.Analyze(bundle)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	switch *format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "text":
		renderText(os.Stdout, report)
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported format %q\n", *format)
		os.Exit(2)
	}
}

func readBundle(path string) (check.Bundle, error) {
	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return check.Bundle{}, err
		}
		defer file.Close()
		reader = file
	}

	var bundle check.Bundle
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return check.Bundle{}, fmt.Errorf("decode evidence JSON: %w", err)
	}
	return bundle, nil
}
