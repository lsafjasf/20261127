package diag

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDevicesObjectAndArray(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.json")
	if err := os.WriteFile(one, []byte(`{"dolby_vision":true,"profiles":[8]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	devices, err := LoadDevices(one)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Name != "device-1" || !devices[0].DolbyVision {
		t.Fatalf("devices = %+v", devices)
	}

	many := filepath.Join(dir, "many.json")
	if err := os.WriteFile(many, []byte(`[{"name":"a"},{"name":"b"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	devices, err = LoadDevices(many)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[1].Name != "b" {
		t.Fatalf("devices = %+v", devices)
	}
}
