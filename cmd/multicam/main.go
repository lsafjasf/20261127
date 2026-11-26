package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"multicam"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "multicam: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	inputPath := flag.String("in", "-", "input JSON path, or - for stdin")
	outputPath := flag.String("out", "-", "output JSON path, or - for stdout")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	flag.Parse()

	input, closeInput, err := openInput(*inputPath)
	if err != nil {
		return err
	}
	defer closeInput()

	project, err := multicam.DecodeProject(input)
	if err != nil {
		return err
	}
	report, err := multicam.Analyze(project)
	if err != nil {
		return fmt.Errorf("validate project: %w", err)
	}

	output, closeOutput, err := openOutput(*outputPath)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	if *pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		_ = closeOutput()
		return fmt.Errorf("encode report: %w", err)
	}
	if err := closeOutput(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	return nil
}

func openInput(path string) (io.Reader, func() error, error) {
	if path == "-" {
		return os.Stdin, func() error { return nil }, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open input: %w", err)
	}
	return file, file.Close, nil
}

func openOutput(path string) (io.Writer, func() error, error) {
	if path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create output: %w", err)
	}
	return file, file.Close, nil
}
