package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"camsync/internal/align"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "camsync:", err)
		os.Exit(2)
	}
}

func run() error {
	inputPath := flag.String("in", "-", "evidence manifest path, or - for stdin")
	outputPath := flag.String("out", "-", "report path, or - for stdout")
	format := flag.String("format", "json", "output format: json or text")
	pretty := flag.Bool("pretty", true, "indent JSON output")
	flag.Parse()

	reader, closeInput, err := openInput(*inputPath)
	if err != nil {
		return err
	}
	defer closeInput()

	var session align.Session
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&session); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("manifest contains trailing data")
	}

	report, err := align.Solve(session)
	if err != nil {
		return fmt.Errorf("align: %w", err)
	}
	writer, closeOutput, err := openOutput(*outputPath)
	if err != nil {
		return err
	}
	defer closeOutput()

	switch *format {
	case "json":
		encoder := json.NewEncoder(writer)
		if *pretty {
			encoder.SetIndent("", "  ")
		}
		if err := encoder.Encode(report); err != nil {
			return fmt.Errorf("encode report: %w", err)
		}
	case "text":
		if err := align.WriteText(writer, report); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	default:
		return fmt.Errorf("unknown format %q", *format)
	}
	return nil
}

func openInput(path string) (io.Reader, func() error, error) {
	if path == "-" {
		return os.Stdin, func() error { return nil }, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func openOutput(path string) (io.Writer, func() error, error) {
	if path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}
