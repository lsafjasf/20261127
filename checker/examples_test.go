package checker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mediaqacontract/contract"
	"mediaqacontract/media"
)

func TestExampleProbes(t *testing.T) {
	root := filepath.Join("..", "examples")

	contractData, err := os.ReadFile(filepath.Join(root, "contracts", "streaming-hd.json"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	spec, err := contract.Load(contractData)
	if err != nil {
		t.Fatalf("load contract: %v", err)
	}

	cases := []struct {
		probeFile string
		want      Status
	}{
		{"good.json", StatusPass},
		{"failing.json", StatusFail},
		{"undecidable.json", StatusUnknown},
	}
	for _, tc := range cases {
		data, err := os.ReadFile(filepath.Join(root, "probes", tc.probeFile))
		if err != nil {
			t.Fatalf("read %s: %v", tc.probeFile, err)
		}
		var probe media.Probe
		if err := json.Unmarshal(data, &probe); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.probeFile, err)
		}
		if got := Verify(spec, &probe).Overall; got != tc.want {
			t.Fatalf("%s overall = %s, want %s", tc.probeFile, got, tc.want)
		}
	}
}
