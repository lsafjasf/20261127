package diag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type DeviceCapabilities struct {
	Name                       string   `json:"name"`
	DolbyVision                bool     `json:"dolby_vision"`
	Profiles                   []int    `json:"profiles"`
	MaxLevel                   int      `json:"max_level"`
	CompatibilityIDs           []int    `json:"compatibility_ids"`
	SupportsEnhancementLayer   bool     `json:"supports_enhancement_layer"`
	SupportsCompressedMetadata bool     `json:"supports_compressed_metadata"`
	SampleEntries              []string `json:"sample_entries"`
	BaseFormats                []string `json:"base_formats"`
	HEVCMain10                 bool     `json:"hevc_main10"`
	MaxHEVCLevelIDC            int      `json:"max_hevc_level_idc"`
	DVEntryBaseFallback        bool     `json:"dv_entry_base_fallback"`
}

func LoadDevices(path string) ([]DeviceCapabilities, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("capabilities file is empty")
	}
	var devices []DeviceCapabilities
	if data[0] == '[' {
		if err := json.Unmarshal(data, &devices); err != nil {
			return nil, err
		}
	} else {
		var one DeviceCapabilities
		if err := json.Unmarshal(data, &one); err != nil {
			return nil, err
		}
		devices = append(devices, one)
	}
	for i := range devices {
		devices[i].Name = strings.TrimSpace(devices[i].Name)
		if devices[i].Name == "" {
			devices[i].Name = fmt.Sprintf("device-%d", i+1)
		}
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("capabilities file contains no devices")
	}
	return devices, nil
}

func containsInt(values []int, v int) bool {
	if len(values) == 0 {
		return true
	}
	for _, item := range values {
		if item == v {
			return true
		}
	}
	return false
}

func containsString(values []string, v string) bool {
	for _, item := range values {
		if strings.EqualFold(item, v) {
			return true
		}
	}
	return false
}
