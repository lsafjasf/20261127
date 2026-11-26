package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lfecheck/internal/audio"
)

func runRepair(args []string) error {
	fs := flag.NewFlagSet("repair", flag.ContinueOnError)
	var opts commonOptions
	var input, output, reportPath, evidencePath string
	var fade time.Duration
	addCommonFlags(fs, &opts)
	fs.StringVar(&input, "in", "", "input multichannel WAV")
	fs.StringVar(&output, "out", "", "repaired multichannel WAV")
	fs.StringVar(&reportPath, "report", "", "before/after JSON (default <out>.report.json)")
	fs.StringVar(&evidencePath, "evidence-csv", "", "sample evidence CSV (default <out>.evidence.csv)")
	fs.DurationVar(&fade, "fade", 10*time.Millisecond, "cancellation edge fade")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if input == "" || output == "" {
		return fmt.Errorf("repair requires --in and --out")
	}
	if reportPath == "" {
		reportPath = defaultReport(output, ".report.json")
	}
	if evidencePath == "" {
		evidencePath = defaultReport(output, ".evidence.csv")
	}
	program, err := audio.ReadWAV(input)
	if err != nil {
		return err
	}
	cfg, err := opts.config(program)
	if err != nil {
		return err
	}
	result, err := audio.Repair(program, audio.RepairConfig{
		Analysis:        cfg,
		FadeSec:         fade.Seconds(),
		EvidenceSamples: 48,
	})
	if err != nil {
		return err
	}
	if dir := filepath.Dir(output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(output, result.Encoded, 0o644); err != nil {
		return err
	}
	result.Report.Before.SourceFile = input
	result.Report.After.SourceFile = output
	if err := writeEvidenceCSV(evidencePath, result.Report.Evidence); err != nil {
		return err
	}
	if err := writeJSON(reportPath, result.Report); err != nil {
		return err
	}
	fmt.Println("before:")
	fmt.Println(indent(result.Report.Before.String()))
	fmt.Println("after:")
	fmt.Println(indent(result.Report.After.String()))
	v := result.Report.Verification
	fmt.Printf("repair: %s from %s, low band %.2f -> %.2f dBFS (%.2f dB), corr %.3f -> %.3f\n",
		v.SourceName, v.DialogueName, v.LowEnergyBeforeDBFS, v.LowEnergyAfterDBFS,
		v.LowEnergyReductionDB, v.CorrelationBefore, v.CorrelationAfter)
	fmt.Printf("dialogue band delta: %.5f dB, evidence samples: %d\n",
		v.DialogueBandDeltaDB, len(result.Report.Evidence))
	fmt.Printf("output: %s\nreport: %s\nevidence: %s\n", output, reportPath, evidencePath)
	return nil
}

func writeEvidenceCSV(path string, rows []audio.SampleEvidence) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{
		"event_index", "region", "sample", "time_sec", "dialogue_before",
		"source_low_aligned", "removed", "dialogue_after",
		"dialogue_before_pcm16", "removed_pcm16", "dialogue_after_pcm16",
	}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write([]string{
			strconv.Itoa(row.EventIndex),
			row.Region,
			strconv.Itoa(row.Sample),
			fmt.Sprintf("%.9f", row.TimeSec),
			fmt.Sprintf("%.9f", row.Before),
			fmt.Sprintf("%.9f", row.ReferenceLow),
			fmt.Sprintf("%.9f", row.Removed),
			fmt.Sprintf("%.9f", row.After),
			strconv.Itoa(row.BeforePCM16),
			strconv.Itoa(row.RemovedPCM16),
			strconv.Itoa(row.AfterPCM16),
		}); err != nil {
			return err
		}
	}
	return w.Error()
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
