package matroska

import (
	"encoding/json"
	"os"
	"strings"

	"mkvidentity/identity"
)

func LoadInventory(path string) (*identity.Inventory, error) {
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var inv identity.Inventory
		if err := json.Unmarshal(data, &inv); err != nil {
			return nil, err
		}
		if inv.TimecodeScaleNS == 0 {
			inv.TimecodeScaleNS = defaultTimecodeScaleNS
		}
		return &inv, nil
	}
	return ReadInventoryPath(path)
}

func SaveInventoryJSON(path string, inv *identity.Inventory) error {
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
