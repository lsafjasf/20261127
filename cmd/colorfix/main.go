package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"colorfix/internal/analyze"
	"colorfix/internal/pipeline"
)

var mediaExt = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".mkv": true,
	".avi": true, ".mpg": true, ".mpeg": true, ".m2v": true,
	".ts": true, ".m2ts": true, ".webm": true, ".wmv": true,
	".flv": true, ".vob": true,
}

func main() {
	var (
		outDir        = flag.String("out", "colorfix-out", "output directory")
		ffmpeg        = flag.String("ffmpeg", "ffmpeg", "ffmpeg executable")
		ffprobe       = flag.String("ffprobe", "ffprobe", "ffprobe executable")
		openSeconds   = flag.Float64("open-seconds", 12, "dense scan window at the start")
		openFPS       = flag.Float64("open-fps", 2, "sampling rate in the opening scan")
		fallbackCount = flag.Int("fallback-count", 16, "samples across the clip if no bars are found at the head")
		skipRelabel   = flag.Bool("skip-relabel", false, "analyze and render only; do not write corrected media")
	)
	maxBarResidual := flag.Float64("max-bar-residual", 0.075, "maximum RMS residual for a trusted color-bar fit")
	minCoverage := flag.Float64("min-bar-coverage", 0.70, "minimum fraction of bars within tolerance")
	minMargin := flag.Float64("min-residual-margin", 0.015, "minimum absolute residual margin for a unique decision")
	minRatio := flag.Float64("min-residual-ratio", 1.60, "minimum loser/winner residual ratio for a unique decision")
	flag.Usage = usage
	flag.Parse()

	inputs, err := collectInputs(flag.Args(), *outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "colorfix:", err)
		os.Exit(2)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "colorfix:", err)
		os.Exit(2)
	}
	opts := pipeline.Options{
		FFmpeg: *ffmpeg, FFprobe: *ffprobe, OutDir: *outDir,
		OpenSeconds: *openSeconds, OpenFPS: *openFPS, FallbackCount: *fallbackCount,
		SkipRelabel: *skipRelabel,
		Decision: analyze.DecisionOptions{
			MaxBarResidual: *maxBarResidual, MinCoverage: *minCoverage,
			MinStructure: 0.70, MinMargin: *minMargin, MinResidualRatio: *minRatio,
		},
	}
	var reports []pipeline.FileReport
	failed := 0
	for i, input := range inputs {
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(inputs), input)
		report, err := pipeline.Process(input, opts)
		if err != nil {
			report.Input = input
			report.Error = err.Error()
			failed++
		}
		reports = append(reports, report)
	}
	if err := pipeline.WriteReport(*outDir, reports); err != nil {
		fmt.Fprintln(os.Stderr, "colorfix: write report:", err)
		os.Exit(2)
	}
	fmt.Printf("reports: %s\n", filepath.Join(*outDir, "report.md"))
	fmt.Printf("frames:  %s\n", filepath.Join(*outDir, "index.html"))
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "colorfix: %d file(s) failed\n", failed)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: colorfix [flags] <video-or-directory>...\n\n")
	fmt.Fprintf(os.Stderr, "Samples color bars, scores BT.601 and BT.709, and retains both\n")
	fmt.Fprintf(os.Stderr, "candidates whenever the measured evidence is not decisive.\n\n")
	flag.PrintDefaults()
}

func collectInputs(args []string, outDir string) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no input supplied")
	}
	outAbs, _ := filepath.Abs(outDir)
	seen := make(map[string]bool)
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			abs, _ := filepath.Abs(arg)
			if !seen[abs] {
				seen[abs] = true
				files = append(files, abs)
			}
			continue
		}
		err = filepath.WalkDir(arg, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				abs, _ := filepath.Abs(path)
				if abs == outAbs {
					return filepath.SkipDir
				}
				return nil
			}
			if !mediaExt[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			abs, _ := filepath.Abs(path)
			if !seen[abs] {
				seen[abs] = true
				files = append(files, abs)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no supported video files found")
	}
	return files, nil
}
