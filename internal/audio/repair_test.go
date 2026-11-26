package audio_test

import (
	"math"
	"testing"

	"lfecheck/internal/audio"
	"lfecheck/internal/fixture"
)

func TestRepairReducesLeakWithSampleEvidence(t *testing.T) {
	program := fixture.Program(48000)
	cfg := audio.DefaultRepairConfig()
	cfg.Analysis.DialogueChannel = 2
	result, err := audio.Repair(program, cfg)
	if err != nil {
		t.Fatal(err)
	}
	report := result.Report
	if report.Before.SourceName != "LFE" {
		t.Fatalf("before source = %s, want LFE", report.Before.SourceName)
	}
	if len(report.Events) == 0 {
		t.Fatal("no repair event")
	}
	for _, event := range report.Events {
		if event.LowEnergyReductionDB < 10 {
			t.Errorf("only %.2f dB low-band reduction", event.LowEnergyReductionDB)
		}
		if math.Abs(event.AfterCorrelation) >= 0.35 {
			t.Errorf("after correlation %.3f remains too high", event.AfterCorrelation)
		}
		if math.Abs(event.DialogueBandDeltaDB) > 0.15 {
			t.Errorf("dialogue band changed by %.3f dB", event.DialogueBandDeltaDB)
		}
	}
	if report.Verification.LeakAfterSec > 0.12 {
		t.Errorf("leak after repair = %.3fs", report.Verification.LeakAfterSec)
	}
	if len(report.Evidence) == 0 {
		t.Fatal("no sample-level evidence")
	}
	for _, row := range report.Evidence {
		actualAfter := result.Output.Samples[report.Verification.DialogueIndex][row.Sample]
		if row.After != actualAfter {
			t.Errorf("sample %d evidence after=%g, output=%g", row.Sample, row.After, actualAfter)
		}
		if math.Abs(row.Before-row.After-row.Removed) > 1e-12 {
			t.Errorf("sample %d evidence is inconsistent", row.Sample)
		}
	}
}

func TestRepairLeavesNonDialogueChannelsUntouched(t *testing.T) {
	program := fixture.Program(48000)
	cfg := audio.DefaultRepairConfig()
	cfg.Analysis.DialogueChannel = 2
	result, err := audio.Repair(program, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for ch := range program.Samples {
		if ch == reportDialogueChannel(result.Report) {
			continue
		}
		for i := range program.Samples[ch] {
			if math.Abs(program.Samples[ch][i]-result.Output.Samples[ch][i]) > 1e-7 {
				t.Fatalf("channel %d sample %d changed", ch, i)
			}
		}
	}
}

func reportDialogueChannel(report audio.RepairReport) int {
	return report.Verification.DialogueIndex
}
