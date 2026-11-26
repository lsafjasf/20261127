// blacklift removes cropped-lens black fog with spatially local, noise-aware
// black-level compensation while protecting intentional lifted-black grades.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"blacklift/internal/compensate"
	"blacklift/internal/estim"
	"blacklift/internal/frameio"
	"blacklift/internal/report"
	"blacklift/internal/style"
)

type config struct {
	inDir       string
	outDir      string
	csvPath     string
	jsonPath    string
	dryRun      bool
	cols        int
	rows        int
	minTile     int
	maxComp     float64
	noiseK      float64
	styleScale  float64
	styleThresh float64
	smoothAlpha float64
	voteWindow  int
	forceStyle  string
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "blacklift:", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.inDir, "in", "", "input directory of PNG/PPM frames")
	flag.StringVar(&c.outDir, "out", "", "output directory for corrected frames")
	flag.StringVar(&c.csvPath, "csv", "blacklift_report.csv", "per-frame CSV report path")
	flag.StringVar(&c.jsonPath, "json", "blacklift_report.json", "summary JSON report path")
	flag.BoolVar(&c.dryRun, "dry-run", false, "analyze and report only; write no corrected frames")
	flag.IntVar(&c.cols, "cols", 16, "estimation grid columns")
	flag.IntVar(&c.rows, "rows", 9, "estimation grid rows")
	flag.IntVar(&c.minTile, "min-tile", 24, "minimum tile edge in pixels")
	flag.Float64Var(&c.maxComp, "max-comp-8bit", 16, "hard cap on subtraction (8-bit levels)")
	flag.Float64Var(&c.noiseK, "noise-k", 1.5, "noise-floor guard: keep k*sigma")
	flag.Float64Var(&c.styleScale, "style-scale", 0, "compensation scale for intentional-lift shots")
	flag.Float64Var(&c.styleThresh, "style-thresh", 0.62, "style-gate score threshold")
	flag.Float64Var(&c.smoothAlpha, "smooth-alpha", 0.65, "temporal EMA alpha (1 disables)")
	flag.IntVar(&c.voteWindow, "vote-window", 5, "temporal style-vote window (frames)")
	flag.StringVar(&c.forceStyle, "force-style", "auto", "auto|lift|normal overrides style detection")
	flag.Parse()
	return c
}

type frameState struct {
	frame *frameio.Frame
	mask  *estim.Mask
	sigma float64
	feat  style.Features
	score float64
	raw   []float64
}

func run(cfg config) error {
	if cfg.inDir == "" {
		return fmt.Errorf("-in is required")
	}
	paths, err := frameio.ListFrames(cfg.inDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no supported frames in %s", cfg.inDir)
	}

	opts := compensate.Options{
		MaxComp:    cfg.maxComp / 255.0,
		NoiseK:     cfg.noiseK,
		StyleScale: cfg.styleScale,
		BlackGuard: 2.0 / 255.0,
	}
	sp := style.DefaultParams()

	states := make([]*frameState, len(paths))
	scores := make([]float64, len(paths))

	// Pass 1: load, estimate local black level, noise and style features.
	for idx, p := range paths {
		fr, err := frameio.Load(p)
		if err != nil {
			return err
		}
		g := estim.BuildGrid(fr.Width, fr.Height, cfg.cols, cfg.rows, cfg.minTile)
		g.Stats(fr.Y)
		sigma := g.FrameSigma()
		mask := estim.FitMask(fr.Y, fr.Width, fr.Height, 24)
		feat := style.ExtractFeatures(fr.Y, sigma, mask.Spread)
		_, score := style.Classify(feat, sp, cfg.styleThresh)
		raw := compensate.BuildCompMap(mask, fr.Width, fr.Height)
		states[idx] = &frameState{frame: fr, mask: mask, sigma: sigma, feat: feat, score: score, raw: raw}
		scores[idx] = score
	}

	liftFlags := style.TemporalVote(scores, cfg.styleThresh, cfg.voteWindow)
	for i := range liftFlags {
		switch cfg.forceStyle {
		case "lift":
			liftFlags[i] = true
		case "normal":
			liftFlags[i] = false
		}
	}

	// Temporal smoothing of raw maps, then bounded application.
	maps := make([][]float64, len(states))
	for i, st := range states {
		maps[i] = st.raw
	}
	compensate.SmoothMaps(maps, cfg.smoothAlpha)

	results := make([]*compensate.FrameResult, len(states))
	if !cfg.dryRun && cfg.outDir != "" {
		if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
			return err
		}
	}

	for i, st := range states {
		res := compensate.Apply(st.frame, maps[i], st.sigma, liftFlags[i], opts)
		res.Index = i
		res.StyleLift = liftFlags[i]
		res.StyleScore = st.score
		res.MaskBase = st.mask.Base
		res.MaskCorner = st.mask.Corner
		res.MaskSpread = st.mask.Spread
		res.MaskRSquared = st.mask.RSquared
		results[i] = res

		if !cfg.dryRun && cfg.outDir != "" {
			out := filepath.Join(cfg.outDir, filepath.Base(st.frame.Path))
			if err := st.frame.SavePNG(out); err != nil {
				return err
			}
		}
	}

	if err := report.WriteCSV(cfg.csvPath, results); err != nil {
		return err
	}
	if err := report.WriteJSON(cfg.jsonPath, results); err != nil {
		return err
	}

	sum := report.BuildSummary(results)
	fmt.Printf("processed %d frames | lift-style: %d | mean applied %.2f 8-bit | max %.2f | new luma clip px %d (%.4f%%)\n",
		sum.Frames, sum.StyleLiftFrames, sum.MeanApplied8bit, sum.MaxApplied8bit,
		sum.TotalNewClipLuma, 100*sum.MeanNewClipFrac)
	fmt.Printf("reports: %s , %s\n", cfg.csvPath, cfg.jsonPath)
	return nil
}
