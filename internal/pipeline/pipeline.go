package pipeline

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"colorfix/internal/analyze"
	"colorfix/internal/color"
	"colorfix/internal/label"
	"colorfix/internal/media"
	"colorfix/internal/video"
)

type Options struct {
	FFmpeg        string
	FFprobe       string
	OutDir        string
	OpenSeconds   float64
	OpenFPS       float64
	FallbackCount int
	Decision      analyze.DecisionOptions
	SkipRelabel   bool
}

type FrameArtifacts struct {
	BT709   string `json:"bt709"`
	BT601   string `json:"bt601"`
	Compare string `json:"compare"`
}

type ProbeReport struct {
	FormatName     string  `json:"format_name"`
	Duration       float64 `json:"duration_seconds"`
	Codec          string  `json:"codec"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	PixFmt         string  `json:"pix_fmt"`
	ColorSpace     string  `json:"declared_color_space"`
	ColorPrimaries string  `json:"color_primaries"`
	ColorTransfer  string  `json:"color_transfer"`
	ColorRange     string  `json:"color_range"`
	MatrixSource   string  `json:"matrix_source"`
}

type FramePair struct {
	Label  string         `json:"label"`
	Time   float64        `json:"time"`
	Frames FrameArtifacts `json:"frames"`
}

type FileReport struct {
	ID            string               `json:"id"`
	Input         string               `json:"input"`
	Error         string               `json:"error,omitempty"`
	OutputDir     string               `json:"output_dir"`
	Probe         ProbeReport          `json:"probe"`
	Bars          analyze.BarAnalysis  `json:"color_bars"`
	Decision      analyze.Decision     `json:"decision"`
	Labels        label.LabelSet       `json:"labels"`
	BarsFrame     *FramePair           `json:"bars_frame,omitempty"`
	Content       *FramePair           `json:"content_frame,omitempty"`
	Fixed         *media.RelabelResult `json:"fixed_media,omitempty"`
	MediaError    string               `json:"media_error,omitempty"`
	FramesScanned int                  `json:"frames_scanned"`
	FallbackScan  bool                 `json:"fallback_scan"`
}

func Process(input string, opts Options) (FileReport, error) {
	probe, err := video.ProbeFile(opts.FFprobe, input)
	if err != nil {
		return FileReport{}, err
	}
	id := outputID(input)
	outDir := filepath.Join(opts.OutDir, id)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return FileReport{}, err
	}
	report := FileReport{
		ID: id, Input: input, OutputDir: outDir,
		Probe: probeReport(probe),
	}
	if opts.OpenFPS <= 0 {
		opts.OpenFPS = 2
	}
	if opts.OpenSeconds <= 0 {
		opts.OpenSeconds = 12
	}
	frames, bestBar, bestChroma := scanOpening(input, probe, opts)
	report.FramesScanned = len(frames)
	if len(frames) == 0 {
		return report, fmt.Errorf("no frames decoded from %s", input)
	}
	if !bestBar.Found && probe.Duration > opts.OpenSeconds {
		if fallbackFrames, fallbackBar, fallbackChroma := scanFallback(input, probe, opts); len(fallbackFrames) > 0 {
			frames, bestBar, bestChroma = fallbackFrames, fallbackBar, fallbackChroma
			report.FramesScanned += len(frames)
			report.FallbackScan = true
		}
	}
	if bestBar.Y == 0 && bestBar.Samples == nil {
		bestBar = analyze.AnalyzeBars(frames[0])
	}
	report.Bars = bestBar
	report.Decision = analyze.Decide(bestBar, probe.Matrix, probe.MatrixName, opts.Decision)
	var cands []color.Matrix
	for _, c := range report.Decision.Candidates {
		cands = append(cands, c.Matrix)
	}
	report.Labels = label.Build(probe, report.Decision.Selected, cands)

	content := bestChroma
	if probe.Duration > 4 {
		t := math.Max(1, probe.Duration*0.35)
		t = math.Min(t, probe.Duration-0.5)
		if contentFrame, err := extractAt(input, probe, opts.FFmpeg, t); err == nil {
			content = contentFrame
		}
	}
	left, right, leftText, rightText := comparisonLabels(probe, report.Decision)
	barFrame := frameAt(frames, bestBar.FrameTime)
	if pair, err := writePairFrames(outDir, "bars", bestBar.FrameTime, barFrame, left, right, leftText, rightText); err == nil {
		report.BarsFrame = pair
	}
	if pair, err := writePairFrames(outDir, "content", content.Time, content, left, right, leftText, rightText); err == nil {
		report.Content = pair
	}

	if !opts.SkipRelabel && report.Decision.Unique && (report.Decision.Status == analyze.StatusRelabeled || report.Decision.Status == analyze.StatusLabeled) {
		fixedPath, err := relabelMedia(input, outDir, probe, report.Decision.Selected, opts)
		if err != nil {
			report.MediaError = err.Error()
		} else {
			report.Fixed = &fixedPath
		}
	}
	if err := writeSidecar(filepath.Join(outDir, "label.json"), report); err != nil {
		return report, err
	}
	return report, nil
}

func scanOpening(input string, probe video.Probe, opts Options) ([]*video.Frame, analyze.BarAnalysis, *video.Frame) {
	frames, err := extractWindow(input, probe, opts.FFmpeg, 0, opts.OpenSeconds, opts.OpenFPS, int(math.Ceil(opts.OpenSeconds*opts.OpenFPS)))
	if err != nil {
		return nil, analyze.BarAnalysis{}, nil
	}
	return analyzeFrames(frames)
}

func scanFallback(input string, probe video.Probe, opts Options) ([]*video.Frame, analyze.BarAnalysis, *video.Frame) {
	if opts.FallbackCount <= 0 {
		opts.FallbackCount = 16
	}
	seconds := probe.Duration
	fps := float64(opts.FallbackCount) / seconds
	frames, err := extractWindow(input, probe, opts.FFmpeg, 0, seconds, fps, opts.FallbackCount)
	if err != nil {
		return nil, analyze.BarAnalysis{}, nil
	}
	return analyzeFrames(frames)
}

func analyzeFrames(frames []*video.Frame) ([]*video.Frame, analyze.BarAnalysis, *video.Frame) {
	bestBar := analyze.BarAnalysis{}
	bestScore := math.Inf(1)
	var bestChroma *video.Frame
	chroma := -1.0
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		a := analyze.AnalyzeBars(frame)
		score := math.Min(a.Score(color.BT601).Residual, a.Score(color.BT709).Residual)
		if score < bestScore {
			bestScore = score
			bestBar = a
		}
		if c := frame.MeanChroma(); c > chroma {
			chroma = c
			bestChroma = frame
		}
	}
	return frames, bestBar, bestChroma
}
