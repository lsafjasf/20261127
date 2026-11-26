package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	aperturemap "aperturemap"
)

type scenarioReport struct {
	ID     string                `json:"id"`
	Title  string                `json:"title,omitempty"`
	Spec   aperturemap.Spec      `json:"spec"`
	OK     bool                  `json:"ok"`
	Result *aperturemap.Resolved `json:"result,omitempty"`
	Error  string                `json:"error,omitempty"`
	Kind   string                `json:"kind,omitempty"`
}

type demoReport struct {
	Combinations   []scenarioReport `json:"combinations"`
	PlayerMismatch []scenarioReport `json:"player_mismatch"`
	Rejections     []scenarioReport `json:"rejections"`
}

func main() {
	input := flag.String("in", "", "path to a geometry spec JSON file; - means stdin")
	flag.Parse()

	if *input != "" {
		if err := resolveFile(*input, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "aperturemap:", err)
			os.Exit(1)
		}
		return
	}

	if err := writeDemo(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "aperturemap:", err)
		os.Exit(1)
	}
}

func resolveFile(path string, out io.Writer) error {
	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}

	var spec aperturemap.Spec
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return fmt.Errorf("decode spec: %w", err)
	}

	resolved, err := aperturemap.Resolve(spec)
	encoder := newEncoder(out)
	if err != nil {
		report := map[string]any{
			"ok":    false,
			"kind":  errorKind(err),
			"error": err.Error(),
		}
		_ = encoder.Encode(report)
		return err
	}
	return encoder.Encode(map[string]any{"ok": true, "result": resolved})
}

func writeDemo(out io.Writer) error {
	report := demoReport{
		Combinations:   make([]scenarioReport, 0, 4),
		PlayerMismatch: make([]scenarioReport, 0, 3),
		Rejections:     make([]scenarioReport, 0, 2),
	}

	report.Combinations = appendReports(report.Combinations, aperturemap.FourCombinations())
	report.PlayerMismatch = appendReports(report.PlayerMismatch, aperturemap.PlayerMismatchExamples())
	report.Rejections = appendReports(report.Rejections, aperturemap.RejectionExamples())

	return newEncoder(out).Encode(report)
}

func appendReports(items []scenarioReport, combos []aperturemap.Combination) []scenarioReport {
	for _, combo := range combos {
		spec := combo.Spec
		if spec.PixelAspectRatio.N == 0 && spec.PixelAspectRatio.D == 0 {
			spec.PixelAspectRatio = aperturemap.MustRatio(1, 1)
		}
		resolved, err := aperturemap.Resolve(combo.Spec)
		item := scenarioReport{
			ID:    combo.ID,
			Title: combo.Title,
			Spec:  spec,
		}
		if err != nil {
			item.OK = false
			item.Error = err.Error()
			item.Kind = errorKind(err)
		} else {
			item.OK = true
			item.Result = resolved
		}
		items = append(items, item)
	}
	return items
}

func newEncoder(out io.Writer) *json.Encoder {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder
}

func errorKind(err error) string {
	switch {
	case errors.Is(err, aperturemap.ErrInconsistentRatio):
		return "inconsistent_ratio"
	case errors.Is(err, aperturemap.ErrApertureOutOfBounds):
		return "aperture_out_of_bounds"
	case errors.Is(err, aperturemap.ErrPresentationImpossible):
		return "presentation_impossible"
	case errors.Is(err, aperturemap.ErrInvalidGeometry):
		return "invalid_geometry"
	default:
		return "unknown"
	}
}
