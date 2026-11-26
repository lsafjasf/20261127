package rangecheck

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// WriteJSON writes the complete machine-readable report.
func WriteJSON(report *Report, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// WriteFrameCSV writes one row per frame, which is the primary frame-by-frame artifact.
func WriteFrameCSV(report *Report, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	header := []string{
		"frame", "pts_seconds", "width", "height", "y_min", "y_max", "y_mean",
		"below_16", "within_16_235", "above_235", "below_ratio", "above_ratio",
		"cb_out_of_range", "cr_out_of_range",
		"out_of_range_pixels", "out_of_range_ratio", "exact_0", "exact_255",
		"region_count", "regions",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, frame := range report.Frames {
		regionSummaries := make([]string, 0, len(frame.Regions))
		for _, r := range frame.Regions {
			regionSummaries = append(regionSummaries, fmt.Sprintf(
				"id=%d,track=%d,%s,%s,rect=%d,%d,%d,%d,px=%d,conf=%.2f",
				r.ID, r.TrackID, r.Kind, r.Source, r.Rect.X, r.Rect.Y, r.Rect.Width, r.Rect.Height,
				r.PixelCount, r.Confidence))
		}
		row := []string{
			strconv.Itoa(frame.Index),
			strconv.FormatFloat(frame.PTSSeconds, 'f', 6, 64),
			strconv.Itoa(frame.Width), strconv.Itoa(frame.Height),
			strconv.Itoa(frame.Y.Min), strconv.Itoa(frame.Y.Max),
			strconv.FormatFloat(frame.Y.Mean, 'f', 3, 64),
			strconv.Itoa(frame.Y.BelowLegal), strconv.Itoa(frame.Y.WithinLegal),
			strconv.Itoa(frame.Y.AboveLegal),
			strconv.FormatFloat(frame.Y.BelowRatio, 'f', 8, 64),
			strconv.FormatFloat(frame.Y.AboveRatio, 'f', 8, 64),
			strconv.Itoa(chromaOutOfRange(frame.Cb)),
			strconv.Itoa(chromaOutOfRange(frame.Cr)),
			strconv.Itoa(frame.OutOfRangePixels),
			strconv.FormatFloat(frame.OutOfRangeRatio, 'f', 8, 64),
			strconv.Itoa(frame.Y.ExactBlack), strconv.Itoa(frame.Y.ExactWhite),
			strconv.Itoa(len(frame.Regions)), strings.Join(regionSummaries, " | "),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func chromaOutOfRange(stats *PlaneStats) int {
	if stats == nil {
		return 0
	}
	return stats.BelowLegal + stats.AboveLegal
}

// WriteMarkdown writes a concise human-review summary with concrete locations.
func WriteMarkdown(report *Report, path string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Limited-range 逐帧质检报告\n\n")
	fmt.Fprintf(&b, "- 输入：`%s`\n", report.Input)
	fmt.Fprintf(&b, "- 帧数：%d；声明帧率：%.6g fps；VFR 分数：%.3f\n", report.FPS.FrameCount, report.FPS.DeclaredFPS, report.FPS.VFRScore)
	fmt.Fprintf(&b, "- 合法码值：Y %d–%d，C %d–%d（%d-bit）\n",
		report.LegalRanges.Luma.Black, report.LegalRanges.Luma.White,
		report.LegalRanges.Chroma.Black, report.LegalRanges.Chroma.White, report.LegalRanges.Bits)
	if report.Metadata.ColorRange != "" {
		fmt.Fprintf(&b, "- 元数据范围：`%s`；编码：`%s`；像素格式：`%s`\n", report.Metadata.ColorRange, report.Metadata.Codec, report.Metadata.PixelFormat)
	}
	b.WriteString("\n## 自动结论\n\n")
	for _, conclusion := range report.Conclusions {
		fmt.Fprintf(&b, "- %s\n", conclusion)
	}

	b.WriteString("\n## 异常段与画面区域\n\n")
	if len(report.Segments) == 0 {
		b.WriteString("未发现连续越界段。\n")
	}
	for _, seg := range report.Segments {
		fmt.Fprintf(&b, "### 帧 %d–%d（%.3f–%.3f 秒）\n\n", seg.FirstFrame, seg.LastFrame, seg.StartSeconds, seg.EndSeconds)
		fmt.Fprintf(&b, "- 分类：`%s`\n", seg.Classification)
		fmt.Fprintf(&b, "- 说明：%s\n", seg.Conclusion)
		fmt.Fprintf(&b, "- 峰值帧：%d，越界比例：%.4f%%\n", seg.PeakFrame, seg.PeakRatio*100)
		for _, trackID := range seg.TrackIDs {
			for _, track := range report.Tracks {
				if track.ID != trackID {
					continue
				}
				r := track.Representative
				fmt.Fprintf(&b, "- 轨迹 #%d `%s` / `%s`：帧 %d–%d，区域 `x=%d,y=%d,w=%d,h=%d`，置信度 %.2f\n",
					track.ID, track.Source, track.Verdict, track.FirstFrame, track.LastFrame,
					r.X, r.Y, r.Width, r.Height, track.BestConfidence)
			}
		}
	}

	if len(report.StyleIntervals) > 0 {
		b.WriteString("\n## 合法风格化墙\n\n")
		for _, interval := range report.StyleIntervals {
			fmt.Fprintf(&b, "- 帧 %d–%d（%.3f–%.3f 秒）：黑墙 %.1f%%，白墙 %.1f%%，越界 %.3f%%；%s\n",
				interval.FirstFrame, interval.LastFrame, interval.StartSeconds, interval.EndSeconds,
				interval.BlackWallRatio*100, interval.WhiteWallRatio*100, interval.OutOfRangeRatio*100,
				interval.Interpretation)
		}
	}

	b.WriteString("\n## 人工复核项\n\n")
	for _, item := range report.ManualChecks {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func addFrame(analyzer *Analyzer, frame Frame, options ProcessOptions) (*FrameReport, error) {
	report, err := analyzer.Add(frame)
	if err != nil {
		return nil, err
	}
	overlayDir := options.Settings.OverlayDir
	if options.Settings.GenerateOverlays && overlayDir == "" {
		overlayDir = "overlays"
	}
	if overlayDir != "" && report.HasOutOfRange && len(report.Regions) > 0 {
		path, err := WriteOverlayPNG(overlayDir, frame, *report)
		if err != nil {
			return nil, err
		}
		report.OverlayPath = path
		if n := len(analyzer.Report.Frames); n > 0 {
			analyzer.Report.Frames[n-1].OverlayPath = path
		}
	}
	if options.OnFrame != nil {
		options.OnFrame(report)
	}
	return report, nil
}

// WriteOverlayPNG renders a luma/RGB reference frame and rectangle annotations.
func WriteOverlayPNG(dir string, frame Frame, report FrameReport) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	img := toRGBA(frame)
	thickness := maxInt(2, frame.Width/450)
	for _, region := range report.Regions {
		lineColor := color.RGBA{R: 40, G: 120, B: 255, A: 255}
		if region.Kind == KindSuperWhite {
			lineColor = color.RGBA{R: 255, G: 55, B: 45, A: 255}
		}
		if region.Source == SourceCodecEdge {
			lineColor = color.RGBA{R: 255, G: 215, B: 0, A: 255}
		}
		drawRect(img, region.Rect, thickness, lineColor)
	}
	path := filepath.Join(dir, fmt.Sprintf("frame_%06d.png", report.Index))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return path, nil
}

func toRGBA(frame Frame) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, frame.Width, frame.Height))
	for y := 0; y < frame.Height; y++ {
		for x := 0; x < frame.Width; x++ {
			var r, g, b byte
			switch frame.Format {
			case FormatRGBA32:
				i := (y*frame.Width + x) * 4
				r, g, b = frame.RGB[i], frame.RGB[i+1], frame.RGB[i+2]
			case FormatRGB24:
				i := (y*frame.Width + x) * 3
				r, g, b = frame.RGB[i], frame.RGB[i+1], frame.RGB[i+2]
			default:
				v := frame.Y[y*frame.StrideY+x]
				r, g, b = v, v, v
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

func drawRect(img *image.RGBA, rect Rect, thickness int, c color.Color) {
	bounds := img.Bounds()
	for t := 0; t < thickness; t++ {
		r := image.Rect(rect.X-t, rect.Y-t, rect.X+rect.Width+t, rect.Y+rect.Height+t)
		for x := r.Min.X; x < r.Max.X; x++ {
			setIfInside(img, x, r.Min.Y, bounds, c)
			setIfInside(img, x, r.Max.Y-1, bounds, c)
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			setIfInside(img, r.Min.X, y, bounds, c)
			setIfInside(img, r.Max.X-1, y, bounds, c)
		}
	}
}

func setIfInside(img *image.RGBA, x, y int, bounds image.Rectangle, c color.Color) {
	p := image.Point{X: x, Y: y}
	if p.In(bounds) {
		img.Set(x, y, c)
	}
}
