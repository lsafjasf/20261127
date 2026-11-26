package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"colorfix/internal/analyze"
	"colorfix/internal/color"
	"colorfix/internal/label"
	"colorfix/internal/media"
	"colorfix/internal/render"
	"colorfix/internal/video"
)

var safeID = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func outputID(input string) string {
	abs, _ := filepath.Abs(input)
	sum := sha256.Sum256([]byte(abs))
	hash := hex.EncodeToString(sum[:])[:8]
	stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	stem = strings.Trim(safeID.ReplaceAllString(stem, "-"), "-._")
	if stem == "" {
		stem = "video"
	}
	if len(stem) > 56 {
		stem = stem[:56]
	}
	return stem + "-" + hash
}

func probeReport(p video.Probe) ProbeReport {
	return ProbeReport{
		FormatName: p.Format.FormatName, Duration: p.Duration,
		Codec: p.Stream.CodecName, Width: p.Stream.Width, Height: p.Stream.Height,
		PixFmt: p.Stream.PixFmt, ColorSpace: p.Stream.ColorSpace,
		ColorPrimaries: p.Stream.ColorPrimaries, ColorTransfer: p.Stream.ColorTransfer,
		ColorRange: string(p.Range), MatrixSource: p.MatrixSource,
	}
}

func extractWindow(input string, probe video.Probe, ffmpeg string, start, seconds, fps float64, maxFrames int) ([]*video.Frame, error) {
	return video.ExtractFrames(input, video.ExtractOptions{
		FFmpeg: ffmpeg, Width: probe.Stream.Width, Height: probe.Stream.Height,
		Range: probe.Range, FPS: fps, Seconds: seconds, MaxFrames: maxFrames,
		Start: start,
	})
}

func extractAt(input string, probe video.Probe, ffmpeg string, at float64) (*video.Frame, error) {
	frames, err := video.ExtractFrames(input, video.ExtractOptions{
		FFmpeg: ffmpeg, Width: probe.Stream.Width, Height: probe.Stream.Height,
		Range: probe.Range, SingleTime: &at,
	})
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frame extracted at %.3fs", at)
	}
	return frames[0], nil
}

func frameAt(frames []*video.Frame, t float64) *video.Frame {
	if len(frames) == 0 {
		return nil
	}
	best := frames[0]
	bestDelta := math.Abs(best.Time - t)
	for _, f := range frames[1:] {
		if f == nil {
			continue
		}
		if d := math.Abs(f.Time - t); d < bestDelta {
			best, bestDelta = f, d
		}
	}
	return best
}

func comparisonLabels(probe video.Probe, decision analyze.Decision) (color.Matrix, color.Matrix, string, string) {
	leftText := "LEFT: BT.709 DECODE"
	rightText := "RIGHT: BT.601 CANDIDATE"
	if probe.Matrix == color.BT709 {
		leftText = "LEFT: TAGGED BT.709"
	}
	if decision.Selected == color.BT601 {
		rightText = "RIGHT: SELECTED BT.601"
	} else if decision.Selected == color.BT709 {
		rightText = "RIGHT: BT.601 ALTERNATIVE"
	}
	return color.BT709, color.BT601, leftText, rightText
}

func writePairFrames(outDir, prefix string, at float64, frame *video.Frame, left, right color.Matrix, leftText, rightText string) (*FramePair, error) {
	if frame == nil {
		return nil, fmt.Errorf("no %s frame available", prefix)
	}
	result := &FramePair{Label: prefix, Time: at}
	leftName := prefix + "-bt709.png"
	rightName := prefix + "-bt601.png"
	if left == color.BT601 {
		leftName, rightName = rightName, leftName
	}
	leftImg := render.Decode(frame, left)
	rightImg := render.Decode(frame, right)
	if err := render.WritePNG(filepath.Join(outDir, leftName), leftImg); err != nil {
		return result, err
	}
	if err := render.WritePNG(filepath.Join(outDir, rightName), rightImg); err != nil {
		return result, err
	}
	compareName := prefix + "-compare.png"
	if err := render.WritePair(filepath.Join(outDir, compareName), frame, left, right, leftText, rightText); err != nil {
		return result, err
	}
	result.Frames = FrameArtifacts{BT709: leftName, BT601: rightName, Compare: compareName}
	if left == color.BT601 {
		result.Frames.BT709, result.Frames.BT601 = rightName, leftName
	}
	return result, nil
}

func relabelMedia(input, outDir string, probe video.Probe, matrix color.Matrix, opts Options) (media.RelabelResult, error) {
	ext := filepath.Ext(input)
	if ext == "" {
		ext = ".mkv"
	}
	outPath := filepath.Join(outDir, "fixed-"+string(matrix)+ext)
	result, err := media.Relabel(opts.FFmpeg, input, outPath, matrix, probe, true)
	if err != nil {
		return media.RelabelResult{}, err
	}
	if verified, err := video.ProbeFile(opts.FFprobe, outPath); err == nil {
		result.VerifiedMatrix = verified.MatrixName
	}
	return result, nil
}

func writeSidecar(path string, report FileReport) error {
	doc := struct {
		Schema     string               `json:"schema"`
		Input      string               `json:"input"`
		Status     string               `json:"status"`
		Unique     bool                 `json:"unique"`
		Selected   string               `json:"selected_matrix,omitempty"`
		Declared   string               `json:"declared_matrix,omitempty"`
		Labels     label.LabelSet       `json:"labels"`
		Evidence   []analyze.Evidence   `json:"evidence"`
		FixedMedia *media.RelabelResult `json:"fixed_media,omitempty"`
		MediaError string               `json:"media_error,omitempty"`
		Frames     []*FramePair         `json:"frames,omitempty"`
	}{
		Schema: "colorfix/labels/v1", Input: report.Input,
		Status: report.Decision.Status, Unique: report.Decision.Unique,
		Selected: string(report.Decision.Selected), Declared: report.Labels.DeclaredName,
		Labels: report.Labels, Evidence: report.Decision.Evidence,
		FixedMedia: report.Fixed, MediaError: report.MediaError,
	}
	if report.BarsFrame != nil {
		doc.Frames = append(doc.Frames, report.BarsFrame)
	}
	if report.Content != nil {
		doc.Frames = append(doc.Frames, report.Content)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
