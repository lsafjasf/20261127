// Package cli contains the command-line frontend for rangecheck.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rangecheck/internal/rangecheck"
)

// Main runs the CLI and terminates the process with a conventional exit code.
func Main() {
	if err := Run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "rangecheck: %v\n", err)
		os.Exit(1)
	}
}

// Run executes one CLI invocation without calling os.Exit.
func Run(args []string) error {
	fs := flag.NewFlagSet("rangecheck", flag.ContinueOnError)
	input := fs.String("input", "", "input MP4/MOV/MKV/WebM, image, frame directory, or raw YUV/RGB dump")
	kind := fs.String("kind", "auto", "input kind: auto|image|raw|video")
	width := fs.Int("width", 0, "raw frame width")
	height := fs.Int("height", 0, "raw frame height")
	pixFmt := fs.String("pix-fmt", "yuv420p", "raw pixel format: yuv420p|yuv444p|gray8|rgb24|rgba32")
	rangeKind := fs.String("range", "tv", "nominal range: tv/limited or full")
	matrix := fs.String("matrix", "bt601", "RGB conversion matrix for image inputs: bt601|bt709|bt2020")
	fpsFlag := fs.String("fps", "", "frame rate rational or number, e.g. 30, 30000/1001, 24000/1001")
	ptsFile := fs.String("pts-file", "", "optional PTS file: one seconds value per line or frame,pts CSV")
	minRegion := fs.Int("min-region", 0, "minimum region area in pixels")
	maxFrames := fs.Int("max-frames", 0, "analyze at most N frames (0 = all)")
	jsonOut := fs.String("json", "rangecheck-report.json", "JSON report output path")
	csvOut := fs.String("csv", "frames.csv", "per-frame CSV output path")
	mdOut := fs.String("markdown", "rangecheck-report.md", "Markdown summary output path")
	overlayDir := fs.String("overlay-dir", "", "write annotated PNGs to this directory")
	overlays := fs.Bool("overlays", false, "write annotated PNGs to ./overlays")
	ffmpegBin := fs.String("ffmpeg", "ffmpeg", "ffmpeg executable used only as raw-frame decoder")
	ffprobeBin := fs.String("ffprobe", "ffprobe", "ffprobe executable used only for metadata/PTS")
	quiet := fs.Bool("quiet", false, "suppress progress output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*input) == "" {
		return fmt.Errorf("--input is required")
	}

	settings := rangecheck.DefaultSettings(*width, *height)
	settings.PixelFormat = rangecheck.PixelFormat(*pixFmt)
	settings.Range = rangecheck.RangeKind(*rangeKind)
	settings.ColorMatrix = *matrix
	if *minRegion > 0 {
		settings.MinRegionPixels = *minRegion
	}
	if *fpsFlag != "" {
		rate, err := parseFPS(*fpsFlag)
		if err != nil {
			return err
		}
		settings.FPS = rate
	}
	if *overlayDir != "" {
		settings.OverlayDir = *overlayDir
		settings.GenerateOverlays = true
	} else if *overlays {
		settings.OverlayDir = "overlays"
		settings.GenerateOverlays = true
	}

	resolvedKind := rangecheck.InputKind(*kind)
	if resolvedKind == rangecheck.InputAuto {
		resolvedKind = rangecheck.DetectInputKind(*input)
	}
	metadata := rangecheck.Metadata{Path: *input}
	var framePTS []float64
	if resolvedKind == rangecheck.InputVideo {
		probed, pts, err := rangecheck.ProbeVideo(*input, *ffprobeBin)
		if err != nil {
			if settings.Width <= 0 || settings.Height <= 0 || settings.FPS.Num == 0 {
				return fmt.Errorf("probe video failed (%v); pass --width/--height/--fps for raw decoding", err)
			}
			fmt.Fprintf(os.Stderr, "rangecheck: ffprobe unavailable, continuing with explicit flags: %v\n", err)
		} else {
			metadata = probed
			metadata.Path = *input
			if settings.Width == 0 {
				settings.Width = probed.Width
				settings.Height = probed.Height
			}
			if settings.FPS.Num == 0 {
				settings.FPS = probed.FrameRate
			}
			framePTS = pts
		}
	}

	analyzer := rangecheck.NewAnalyzer(*input, settings, metadata)
	options := rangecheck.ProcessOptions{
		Kind: resolvedKind, Settings: settings, Metadata: metadata,
		PTSFile: *ptsFile, FramePTS: framePTS, MaxFrames: *maxFrames,
		FFmpegPath: *ffmpegBin, FFprobePath: *ffprobeBin,
	}
	if !*quiet {
		options.OnFrame = func(frame *rangecheck.FrameReport) {
			if frame.Index%50 == 0 {
				fmt.Fprintf(os.Stderr, "\ranalyzed frame %d", frame.Index)
			}
		}
	}
	report, err := rangecheck.ProcessPath(*input, analyzer, options)
	if err != nil {
		return err
	}
	if !*quiet {
		fmt.Fprintln(os.Stderr, "\ranalyzed frame", report.FPS.FrameCount-1)
	}

	if *jsonOut != "" {
		if err := ensureParent(*jsonOut); err != nil {
			return err
		}
		if err := rangecheck.WriteJSON(report, *jsonOut); err != nil {
			return err
		}
	}
	if *csvOut != "" {
		if err := ensureParent(*csvOut); err != nil {
			return err
		}
		if err := rangecheck.WriteFrameCSV(report, *csvOut); err != nil {
			return err
		}
	}
	if *mdOut != "" {
		if err := ensureParent(*mdOut); err != nil {
			return err
		}
		if err := rangecheck.WriteMarkdown(report, *mdOut); err != nil {
			return err
		}
	}
	if !*quiet {
		fmt.Printf("frames=%d segments=%d fps=%.6g json=%s csv=%s md=%s\n",
			report.FPS.FrameCount, len(report.Segments), report.FPS.DeclaredFPS,
			*jsonOut, *csvOut, *mdOut)
	}
	return nil
}

func ensureParent(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func parseFPS(value string) (rangecheck.Rational, error) {
	if strings.Contains(value, "/") {
		r := rangecheck.ParseRational(value)
		if r.Den == 0 {
			return rangecheck.Rational{}, fmt.Errorf("invalid --fps %q", value)
		}
		return r, nil
	}
	var f float64
	if _, err := fmt.Sscanf(value, "%g", &f); err != nil || f <= 0 {
		return rangecheck.Rational{}, fmt.Errorf("invalid --fps %q", value)
	}
	switch value {
	case "23.976", "23.98":
		return rangecheck.Rational{Num: 24000, Den: 1001}, nil
	case "29.97":
		return rangecheck.Rational{Num: 30000, Den: 1001}, nil
	case "59.94":
		return rangecheck.Rational{Num: 60000, Den: 1001}, nil
	}
	if f == float64(int64(f)) {
		return rangecheck.Rational{Num: int64(f), Den: 1}, nil
	}
	return rangecheck.Rational{Num: int64(f * 1000), Den: 1000}, nil
}
