package main

import (
	"clipforensic/internal/analyze"
	"clipforensic/internal/demo"
	"clipforensic/internal/reportio"
	"clipforensic/internal/wav"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "demo":
			runDemo(os.Args[2:])
			return
		case "compare":
			runCompare(os.Args[2:])
			return
		case "analyze":
			runAnalyze(os.Args[2:])
			return
		}
	}
	runAnalyze(os.Args[1:])
}

func runCompare(args []string) {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	masterPath := fs.String("master", "", "pre-encode master WAV")
	encodedPath := fs.String("encoded", "", "platform/encoded audio decoded to WAV")
	frame := fs.Int("codec-frame-samples", 0, "optional codec frame size")
	cfg := analyzeFlags{}
	fs.StringVar(&cfg.report, "report", "compare_report.json", "write JSON report")
	fs.StringVar(&cfg.candidates, "candidates-csv", "compare_candidates.csv", "write candidate CSV")
	fs.StringVar(&cfg.waveCSV, "waveform-csv", "compare_waveform_evidence.csv", "write waveform CSV")
	fs.StringVar(&cfg.svgDir, "svg-dir", "compare_waveforms", "write SVG panels")
	fs.StringVar(&cfg.markdown, "markdown", "compare.md", "write Markdown report")
	fs.Float64Var(&cfg.opts.CeilingThreshold, "ceiling-threshold", 0.98, "near-ceiling threshold")
	fs.Float64Var(&cfg.opts.FlatTolerance, "flat-tolerance", 0.00012, "plateau flat tolerance")
	fs.Float64Var(&cfg.opts.MinConfidence, "min-confidence", 0.55, "minimum confidence")
	_ = fs.Parse(reorderInterspersed(args, nil))
	if *masterPath == "" || *encodedPath == "" {
		fs.Usage()
		os.Exit(2)
	}
	master, err := wav.ReadFile(*masterPath)
	if err != nil {
		fatal(err)
	}
	encoded, err := wav.ReadFile(*encodedPath)
	if err != nil {
		fatal(err)
	}
	report, err := analyze.Compare(master, encoded, *masterPath, *encodedPath, cfg.opts)
	if err != nil {
		fatal(err)
	}
	if *frame > 0 {
		attachGrid(report, *frame)
	}
	if err := writeOutputs(cfg, report); err != nil {
		fatal(err)
	}
	printText(report)
}

type analyzeFlags struct {
	report     string
	candidates string
	waveCSV    string
	svgDir     string
	markdown   string
	asJSON     bool
	opts       analyze.Options
}

func runAnalyze(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	var cfg analyzeFlags
	fs.StringVar(&cfg.report, "report", "", "write full JSON report")
	fs.StringVar(&cfg.candidates, "candidates-csv", "candidates.csv", "write candidate summary CSV")
	fs.StringVar(&cfg.waveCSV, "waveform-csv", "waveform_evidence.csv", "write per-sample waveform CSV")
	fs.StringVar(&cfg.svgDir, "svg-dir", "waveforms", "write waveform SVG panels")
	fs.StringVar(&cfg.markdown, "markdown", "evidence.md", "write Markdown evidence index")
	fs.BoolVar(&cfg.asJSON, "json", false, "print JSON to stdout")
	fs.Float64Var(&cfg.opts.CeilingThreshold, "ceiling-threshold", 0.98, "near-ceiling amplitude threshold")
	fs.Float64Var(&cfg.opts.FlatTolerance, "flat-tolerance", 0.00012, "maximum sample difference inside a plateau")
	fs.IntVar(&cfg.opts.MinPlateauRun, "min-run", 2, "minimum plateau samples")
	fs.IntVar(&cfg.opts.FFTSize, "fft-size", 2048, "FFT size")
	fs.IntVar(&cfg.opts.WaveBefore, "before", 24, "waveform samples before evidence edge")
	fs.IntVar(&cfg.opts.WaveAfter, "after", 48, "waveform samples after evidence edge")
	fs.Float64Var(&cfg.opts.MinConfidence, "min-confidence", 0.55, "minimum incident confidence")
	codecFrame := fs.Int("codec-frame-samples", 0, "optional codec frame size for grid alignment (AAC=1024, MP3=1152)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s analyze [flags] input.wav [input2.wav ...]\n", filepath.Base(os.Args[0]))
		fs.PrintDefaults()
	}
	_ = fs.Parse(reorderInterspersed(args, map[string]bool{"json": true}))
	paths := fs.Args()
	if len(paths) == 0 {
		fs.Usage()
		os.Exit(2)
	}

	report, err := analyzePaths(paths, cfg.opts)
	if err != nil {
		fatal(err)
	}
	if *codecFrame > 0 {
		attachGrid(report, *codecFrame)
	}
	if err := writeOutputs(cfg, report); err != nil {
		fatal(err)
	}
	if cfg.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		return
	}
	printText(report)
}

func runDemo(args []string) {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	out := fs.String("out", "artifacts/demo", "demo output directory")
	fs.Parse(args)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	musicPath, guitarPath, err := demo.WriteFixtures(*out)
	if err != nil {
		fatal(err)
	}
	report, err := analyzePaths([]string{musicPath, guitarPath}, analyze.Options{})
	if err != nil {
		fatal(err)
	}
	report.Notes = append(report.Notes, "Demo fixtures are deterministic Go-synthesized WAVs because the workspace contained no user-supplied audio.")
	truthPath := filepath.Join(*out, "demo_truth.json")
	if err := reportio.WriteJSON(truthPath, map[string]any{"windows": demo.Truth()}); err != nil {
		fatal(err)
	}
	cfg := analyzeFlags{
		report:     filepath.Join(*out, "report.json"),
		candidates: filepath.Join(*out, "candidates.csv"),
		waveCSV:    filepath.Join(*out, "waveform_evidence.csv"),
		svgDir:     filepath.Join(*out, "waveforms"),
		markdown:   filepath.Join(*out, "evidence.md"),
	}
	if err := writeOutputs(cfg, report); err != nil {
		fatal(err)
	}
	fmt.Printf("demo written to %s\n", *out)
	printText(report)
}

func analyzePaths(paths []string, supplied analyze.Options) (analyze.Report, error) {
	report := analyze.Report{SchemaVersion: "1.0", Tool: "clipforensic"}
	for _, path := range paths {
		a, err := wav.ReadFile(path)
		if err != nil {
			return report, fmt.Errorf("read %s: %w", path, err)
		}
		opts := supplied
		if opts.CeilingThreshold == 0 {
			opts = analyze.DefaultOptions(a.SampleRate)
		}
		if opts.MergeGapSamples == 0 {
			opts.MergeGapSamples = a.SampleRate * 30 / 1000
		}
		if opts.WaveBefore == 0 {
			opts.WaveBefore = 24
		}
		if opts.WaveAfter == 0 {
			opts.WaveAfter = 48
		}
		fr := analyze.Analyze(a, opts)
		fr.File = path
		report.Files = append(report.Files, fr)
	}
	report.Summary.FilesAnalyzed = len(report.Files)
	for _, f := range report.Files {
		report.Summary.IncidentCandidates += len(f.Candidates)
		for _, c := range f.Artistic {
			if c.Status == "artistic_distortion" {
				report.Summary.ArtisticRegions++
			} else {
				report.Summary.InconclusiveRegions++
			}
		}
	}
	switch {
	case report.Summary.IncidentCandidates > 0:
		report.Verdict = "mastering_or_distribution_clips_detected_see_comparison_requirement"
	case report.Summary.ArtisticRegions > 0:
		report.Verdict = "no_accidental_clip_artistic_distortion_guarded"
	default:
		report.Verdict = "no_clip_incident_detected"
	}
	return report, nil
}

func writeOutputs(cfg analyzeFlags, report analyze.Report) error {
	if cfg.report != "" {
		if err := reportio.WriteJSON(cfg.report, report); err != nil {
			return err
		}
	}
	if cfg.candidates != "" {
		if err := reportio.WriteCandidateCSV(cfg.candidates, report); err != nil {
			return err
		}
	}
	if cfg.waveCSV != "" {
		if err := reportio.WriteWaveformCSV(cfg.waveCSV, report); err != nil {
			return err
		}
	}
	var svgs []string
	if cfg.svgDir != "" {
		var err error
		svgs, err = reportio.WriteSVGs(cfg.svgDir, report)
		if err != nil {
			return err
		}
	}
	if cfg.markdown != "" {
		if err := reportio.WriteMarkdown(cfg.markdown, report, svgs); err != nil {
			return err
		}
	}
	return nil
}

func attachGrid(report analyze.Report, frame int) {
	for fi := range report.Files {
		for ci := range report.Files[fi].Candidates {
			c := &report.Files[fi].Candidates[ci]
			mod := c.EvidenceSample % frame
			distance := mod
			if frame-mod < distance {
				distance = frame - mod
			}
			aligned := 0
			if distance <= maxInt(4, frame/64) {
				aligned = 1
			}
			c.CodecGrid = &analyze.GridEvidence{
				FrameSamples:   frame,
				CandidateCount: 1,
				AlignedCount:   aligned,
				AlignmentRatio: 1 - float64(distance)/float64(frame/2),
			}
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func printText(report analyze.Report) {
	fmt.Printf("Verdict: %s\n", report.Verdict)
	for _, f := range report.Files {
		fmt.Printf("\n%s: %d Hz/%dch %.3fs, peak %.4f (%.2f dBFS), exact-FS=%d plateau-runs=%d\n",
			f.File, f.SampleRate, f.Channels, f.DurationSec, f.Global.PeakAmplitude,
			f.Global.PeakDBFS, f.Global.ExactFullScaleSamples, f.Global.PlateauRunCount)
		for _, c := range f.Candidates {
			fmt.Printf("  INCIDENT %s %.3f-%.3fs conf=%.2f %s\n", c.ID, c.StartSec, c.EndSec, c.Confidence, c.Classification)
			fmt.Printf("    plateau runs=%d longest=%d exactFS=%d steepEdge=%.2f\n",
				c.Plateau.RunCount, c.Plateau.LongestRun, c.Plateau.ExactFullScaleSamples, c.Edge.SteepEdgeRatio)
		}
		for _, c := range f.Artistic {
			fmt.Printf("  GUARDED %s %.3f-%.3fs status=%s class=%s conf=%.2f f0=%.1f H=%.2f\n",
				c.ID, c.StartSec, c.EndSec, c.Status, c.Classification, c.Confidence,
				c.Spectrum.EstimatedF0Hz, c.Spectrum.HarmonicStrength)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func reorderInterspersed(args []string, boolFlags map[string]bool) []string {
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(token) < 2 || token[0] != '-' {
			positional = append(positional, token)
			continue
		}
		flags = append(flags, token)
		name := strings.TrimLeft(token, "-")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		}
		if !strings.Contains(token, "=") && !boolFlags[name] && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, positional...)
}
