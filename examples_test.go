package hdrswitch

import (
	"encoding/json"
	"os"
	"testing"
)

func TestInsideGOPExampleJSON(t *testing.T) {
	data, err := os.ReadFile("examples/hlg_to_pq_inside_gop.json")
	if err != nil {
		t.Fatal(err)
	}
	var evidence Evidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatal("example must fail quality check because switch is inside GOP and player is sticky")
	}
	if report.GOPRisk == nil || !report.GOPRisk.AtRisk {
		t.Fatalf("example GOP risk = %+v", report.GOPRisk)
	}
	if report.Before.LegalRange.Transfer != TransferHLG || report.After.LegalRange.Transfer != TransferPQ {
		t.Fatal("example must classify HLG before and PQ after")
	}
}
