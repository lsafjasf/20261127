package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"lfecheck/internal/audio"
)

func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	var opts commonOptions
	var input, reportPath, framesPath string
	addCommonFlags(fs, &opts)
	fs.StringVar(&input, "in", "", "input multichannel WAV")
	fs.StringVar(&reportPath, "report", "", "analysis JSON path (default <in>.analysis.json)")
	fs.StringVar(&framesPath, "frames-csv", "", "optional per-frame diagnostic CSV")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if input == "" {
		return fmt.Errorf("analyze requires --in")
	}
	if reportPath == "" {
		reportPath = defaultReport(input, ".analysis.json")
	}
	program, err := audio.ReadWAV(input)
	if err != nil {
		return err
	}
	cfg, err := opts.config(program)
	if err != nil {
		return err
	}
	analysis, err := audio.Analyze(program, cfg)
	if err != nil {
		return err
	}
	analysis.Report.SourceFile = input
	if framesPath != "" {
		if err := writeFramesCSV(framesPath, analysis); err != nil {
			return err
		}
	}
	if err := writeJSON(reportPath, analysis.Report); err != nil {
		return err
	}
	fmt.Println(analysis.Report.String())
	printChannelSummary(analysis.Report)
	fmt.Printf("report: %s\n", reportPath)
	if framesPath != "" {
		fmt.Printf("frames: %s\n", framesPath)
	}
	return nil
}

func printChannelSummary(report audio.Report) {
	fmt.Println("channel low-band energy (dBFS):")
	for _, ch := range report.ChannelsSummary {
		fmt.Printf("  %-3s full=%7.2f active-dialogue=%7.2f peak=%7.2f @ %.3fs\n",
			ch.Name, ch.LowEnergyDBFS, ch.DialogueActiveDB, ch.PeakLowFrameDBFS, ch.PeakLowTimeSec)
	}
}

func writeJSON(path string, value any) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func writeFramesCSV(path string, analysis *audio.Analysis) error {
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
	header := []string{
		"index", "start_sample", "end_sample", "start_sec", "end_sec",
		"dialogue_band_dbfs", "dialogue_active", "source_low_dbfs",
		"correlation", "lag_samples", "leak",
	}
	for _, name := range analysis.Report.ChannelNames {
		header = append(header, "low_"+name+"_dbfs")
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, frame := range analysis.Frames {
		row := []string{
			strconv.Itoa(frame.Index),
			strconv.Itoa(frame.StartSample),
			strconv.Itoa(frame.EndSample),
			fmt.Sprintf("%.6f", frame.StartSec),
			fmt.Sprintf("%.6f", frame.EndSec),
			fmt.Sprintf("%.3f", frame.DialogueBandDB),
			strconv.FormatBool(frame.DialogueActive),
			fmt.Sprintf("%.3f", frame.SourceLowDBFS),
			fmt.Sprintf("%.6f", frame.Correlation),
			strconv.Itoa(frame.LagSamples),
			strconv.FormatBool(frame.Leak),
		}
		for _, db := range frame.LowDBFS {
			row = append(row, fmt.Sprintf("%.3f", db))
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}
