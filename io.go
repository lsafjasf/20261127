package hdrframe

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SampleFile is the multi-case container used by samples.json.
type SampleFile struct {
	SchemaVersion int     `json:"schema_version"`
	Samples       []Input `json:"samples"`
}

// LoadInput reads one JSON timeline.
func LoadInput(path string) (Input, error) {
	var input Input
	data, err := os.ReadFile(path)
	if err != nil {
		return input, err
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return input, fmt.Errorf("parse %s: %w", path, err)
	}
	return input, nil
}

// LoadSampleFile reads either {"samples":[...]} or a single timeline.
func LoadSampleFile(path string) (SampleFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SampleFile{}, err
	}
	var file SampleFile
	if err := json.Unmarshal(data, &file); err == nil && len(file.Samples) > 0 {
		if file.SchemaVersion == 0 {
			file.SchemaVersion = 1
		}
		return file, nil
	}
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return SampleFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return SampleFile{SchemaVersion: 1, Samples: []Input{input}}, nil
}

// WriteJSON writes indented JSON and creates parent directories.
func WriteJSON(path string, value any) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// WriteSampleDir writes the three anomaly fixtures and one combined file.
func WriteSampleDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	all := Samples()
	names := []string{"duplicate.json", "missing-frame.json", "sequence-jump.json"}
	for i, sample := range all {
		if err := WriteJSON(filepath.Join(dir, names[i]), sample); err != nil {
			return err
		}
	}
	return WriteJSON(filepath.Join(dir, "samples.json"), SampleFile{SchemaVersion: 1, Samples: all})
}

// WriteText writes a plain-text report.
func WriteText(path string, report *Report) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(report.Text()), 0o644)
}

// EncodeJSON writes an indented report to w.
func EncodeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
