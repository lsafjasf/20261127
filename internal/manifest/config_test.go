package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigResolvesPathsRelativeToConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "delivery.json")
	content := `{
	  "project": {"id": "relative-paths"},
	  "hdr": {"path": "media/hdr.mov"},
	  "sdr": {"path": "media/sdr.mov"},
	  "shared": {
	    "audio": [{"id": "main", "path": "assets/main.m4a"}],
	    "subtitles": [{"id": "zh", "path": "assets/zh.srt"}]
	  },
	  "cuts": {"path": "edit/cuts.json"}
	}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	expected := map[string]string{
		"hdr":      filepath.Join(dir, "media", "hdr.mov"),
		"sdr":      filepath.Join(dir, "media", "sdr.mov"),
		"audio":    filepath.Join(dir, "assets", "main.m4a"),
		"subtitle": filepath.Join(dir, "assets", "zh.srt"),
		"cut list": filepath.Join(dir, "edit", "cuts.json"),
	}
	actual := map[string]string{
		"hdr":      config.HDR.Path,
		"sdr":      config.SDR.Path,
		"audio":    config.Shared.Audio[0].Path,
		"subtitle": config.Shared.Subtitles[0].Path,
		"cut list": config.Cuts.Path,
	}
	for name, expectedPath := range expected {
		if actual[name] != expectedPath {
			t.Errorf("%s path = %q, want %q", name, actual[name], expectedPath)
		}
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "delivery.json")
	content := `{
	  "hdr": {"path": "hdr.mov"},
	  "sdr": {"path": "sdr.mov"},
	  "unexpected": true
	}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(configPath)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestLoadConfigRejectsUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "delivery.json")
	content := `{
	  "schema": "hdr-sdr-delivery-input/v99",
	  "hdr": {"path": "hdr.mov"},
	  "sdr": {"path": "sdr.mov"}
	}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(configPath)
	if err == nil || !strings.Contains(err.Error(), "unsupported input schema") {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	base := Config{
		HDR: VersionConfig{Path: "/media/hdr.mov"},
		SDR: VersionConfig{Path: "/media/sdr.mov"},
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "same media path",
			mutate:  func(c *Config) { c.SDR.Path = c.HDR.Path },
			wantErr: "must be different",
		},
		{
			name: "list mode without source",
			mutate: func(c *Config) {
				c.Cuts.Source = "list"
			},
			wantErr: "requires cuts.path or cuts.points",
		},
		{
			name: "path and points together",
			mutate: func(c *Config) {
				c.Cuts.Path = "cuts.json"
				c.Cuts.Points = []CutPoint{{ID: "C1", TimeMS: 0}}
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "duplicate asset id",
			mutate: func(c *Config) {
				c.Shared.Audio = []SharedAsset{{ID: "shared", Path: "a.m4a"}}
				c.Shared.Subtitles = []SharedAsset{{ID: "shared", Path: "s.srt"}}
			},
			wantErr: "used by both",
		},
		{
			name: "two default audio assets",
			mutate: func(c *Config) {
				c.Shared.Audio = []SharedAsset{
					{ID: "a1", Path: "a1.m4a", Default: boolPointer(true)},
					{ID: "a2", Path: "a2.m4a", Default: boolPointer(true)},
				}
			},
			wantErr: "more than one default",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := base
			test.mutate(&config)
			err := config.Validate()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}
