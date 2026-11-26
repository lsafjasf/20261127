package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"lfecheck/internal/audio"
	"lfecheck/internal/fixture"
)

func runDemo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	outDir := fs.String("out-dir", "artifacts/demo", "demo output directory")
	rate := fs.Int("rate", 48000, "sample rate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rate < 8000 {
		return fmt.Errorf("--rate must be at least 8000")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	programPath := filepath.Join(*outDir, "program.wav")
	outputPath := filepath.Join(*outDir, "program_repaired.wav")
	beforePath := filepath.Join(*outDir, "analysis_before.json")
	afterPath := filepath.Join(*outDir, "analysis_after.json")
	reportPath := filepath.Join(*outDir, "repair_report.json")
	evidencePath := filepath.Join(*outDir, "sample_evidence.csv")
	framesBeforePath := filepath.Join(*outDir, "frames_before.csv")
	framesAfterPath := filepath.Join(*outDir, "frames_after.csv")

	program := fixture.Program(*rate)
	program.Format.Bits = 16
	program.Format.Encoding = audio.PCM
	if err := audio.WriteWAV(programPath, program); err != nil {
		return err
	}
	decoded, err := audio.ReadWAV(programPath)
	if err != nil {
		return err
	}
	cfg := audio.DefaultAnalysisConfig()
	cfg.DialogueChannel = 2
	cfg.SourceChannel = -1
	result, err := audio.Repair(decoded, audio.RepairConfig{
		Analysis:        cfg,
		FadeSec:         0.010,
		EvidenceSamples: 48,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, result.Encoded, 0o644); err != nil {
		return err
	}
	result.Before.Report.SourceFile = programPath
	result.After.Report.SourceFile = outputPath
	if err := writeJSON(beforePath, result.Before.Report); err != nil {
		return err
	}
	if err := writeJSON(afterPath, result.After.Report); err != nil {
		return err
	}
	if err := writeFramesCSV(framesBeforePath, result.Before); err != nil {
		return err
	}
	if err := writeFramesCSV(framesAfterPath, result.After); err != nil {
		return err
	}
	if err := writeEvidenceCSV(evidencePath, result.Report.Evidence); err != nil {
		return err
	}
	if err := writeJSON(reportPath, result.Report); err != nil {
		return err
	}
	v := result.Report.Verification
	fmt.Printf("demo: %s\n", *outDir)
	fmt.Printf("  injected leak: %.3f-%.3fs gain=%.2f delay=%d samples\n",
		fixture.LeakStartSec, fixture.LeakEndSec, fixture.LeakGain, fixture.LeakDelay)
	fmt.Printf("  detected: source=%s events=%d leak=%.3fs\n",
		result.Before.Report.SourceName, len(result.Before.Report.Events), result.Before.Report.LeakTotalSec)
	fmt.Printf("  repaired: low %.2f -> %.2f dBFS, corr %.3f -> %.3f, leak %.3fs\n",
		v.LowEnergyBeforeDBFS, v.LowEnergyAfterDBFS, v.CorrelationBefore, v.CorrelationAfter, v.LeakAfterSec)
	fmt.Printf("  files: program.wav, program_repaired.wav, repair_report.json, sample_evidence.csv, frames_*.csv\n")
	return nil
}
