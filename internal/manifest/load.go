package manifest

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads, parses and validates the single source manifest.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: read %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
