// Package spec loads and validates batch render specifications.
package spec

import (
	"encoding/json"
	"fmt"
	"os"

	"renderbatch/internal/batch"
)

// Load reads a JSON spec and validates its invariants.
func Load(path string) (batch.Spec, error) {
	var s batch.Spec
	raw, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("parse spec: %w", err)
	}
	return s, Validate(s)
}

// Validate checks structural invariants that the engine relies on.
func Validate(s batch.Spec) error {
	if s.Name == "" {
		return fmt.Errorf("spec.name must not be empty")
	}
	if len(s.Files) == 0 {
		return fmt.Errorf("spec.files must not be empty")
	}
	seen := map[string]bool{}
	for i, file := range s.Files {
		if err := batch.ValidatePath(file.Path); err != nil {
			return fmt.Errorf("files[%d]: %w", i, err)
		}
		if seen[file.Path] {
			return fmt.Errorf("files[%d]: duplicate path %q", i, file.Path)
		}
		seen[file.Path] = true
		if file.Fragments <= 0 {
			return fmt.Errorf("files[%d].fragments must be positive", i)
		}
		if file.FragmentSize <= 0 {
			return fmt.Errorf("files[%d].fragment_size must be positive", i)
		}
		minSize := file.FragmentSize * (file.Fragments - 1)
		if file.Size <= minSize {
			return fmt.Errorf(
				"files[%d].size must exceed fragment_size*(fragments-1)=%d", i, minSize)
		}
		last := batch.FragmentLength(file, file.Fragments-1)
		if last <= 0 || last > file.FragmentSize*2 {
			return fmt.Errorf("files[%d]: last fragment length %d is invalid", i, last)
		}
	}
	return nil
}
