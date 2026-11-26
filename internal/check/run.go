package check

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"proalpha/internal/mediaio"
	"proalpha/internal/mediameta"
)

type Options struct {
	SourcePath   string
	VariantPaths []string
	OutputDir    string
	MaxFrames    int
	RepairVideo  bool
}

func Run(options Options) (*Report, error) {
	if options.MaxFrames <= 0 {
		options.MaxFrames = 3
	}
	if options.OutputDir == "" {
		options.OutputDir = "alpha-output"
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return nil, err
	}

	report := &Report{
		SchemaVersion:   "proalpha.report/v1",
		Status:          "pass",
		Variants:        []VariantReport{},
		WrongParameters: []WrongParam{},
		OutputDirectory: options.OutputDir,
	}

	var sourceMeta *mediameta.FileMetadata
	if options.SourcePath != "" {
		meta, err := mediameta.Inspect(options.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("inspect source: %w", err)
		}
		sourceMeta = meta
		fileReport := NewFileReport(meta)
		report.Source = &fileReport
		report.WrongParameters = append(report.WrongParameters, InternalMetadataWrongParams(meta)...)
	}

	targets := options.VariantPaths
	if len(targets) == 0 && options.SourcePath != "" {
		targets = []string{options.SourcePath}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one --source or --variant is required")
	}

	for index, path := range targets {
		variantReport, wrong, err := runVariant(options, sourceMeta, path, index)
		if err != nil {
			return nil, err
		}
		report.Variants = append(report.Variants, *variantReport)
		report.WrongParameters = append(report.WrongParameters, wrong...)
	}

	report.Status, report.Summary = summarize(report.WrongParameters)
	if sourceMeta != nil {
		scriptPath := filepath.Join(options.OutputDir, "repair-from-source.sh")
		videoPath := filepath.Join(options.OutputDir, "repaired-alpha-proxy.mov")
		scriptSource := sourceMeta
		if absoluteSource, err := filepath.Abs(sourceMeta.Path); err == nil {
			absoluteMeta := *sourceMeta
			absoluteMeta.Path = absoluteSource
			scriptSource = &absoluteMeta
		}
		absoluteVideo := videoPath
		if absolutePath, err := filepath.Abs(videoPath); err == nil {
			absoluteVideo = absolutePath
		}
		if err := writeRepairScript(scriptSource, scriptPath, absoluteVideo); err != nil {
			return nil, err
		}
		report.RepairScript = scriptPath
	}
	if options.RepairVideo && sourceMeta != nil {
		repairedPath := filepath.Join(options.OutputDir, "repaired-alpha-proxy.mov")
		if err := repairVideo(sourceMeta, repairedPath); err != nil {
			return nil, err
		}
		report.RepairedVideo = repairedPath
	}
	if err := writeJSON(filepath.Join(options.OutputDir, "report.json"), report); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(options.OutputDir, "wrong-params.json"), report.WrongParameters); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(options.OutputDir, "report.txt"), []byte(RenderText(report)), 0o644); err != nil {
		return nil, err
	}
	return report, nil
}

func runVariant(options Options, sourceMeta *mediameta.FileMetadata, path string, index int) (*VariantReport, []WrongParam, error) {
	meta, err := mediameta.Inspect(path)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect %s: %w", path, err)
	}
	fileReport := NewFileReport(meta)
	variant := &VariantReport{File: fileReport, Frames: []FrameReport{}}
	wrong := []WrongParam{}
	wrong = append(wrong, InternalMetadataWrongParams(meta)...)
	wrong = append(wrong, ComparisonMetadataWrongParams(sourceMeta, meta)...)

	if meta.FFProbe == nil || meta.FFProbe.Width <= 0 || meta.FFProbe.Height <= 0 {
		wrong = append(wrong, WrongParam{
			File: path, Code: "probe_unavailable", Severity: SeverityWarning,
			Parameter: "ffprobe", Message: "pixel analysis skipped because ffprobe/stream dimensions are unavailable",
		})
		return variant, wrong, nil
	}

	width, height := meta.FFProbe.Width, meta.FFProbe.Height
	decodedVariant, err := mediaio.DecodeRGBA64(path, mediaio.DecodeOptions{
		MaxFrames: options.MaxFrames, Width: width, Height: height,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("decode variant pixels %s: %w", path, err)
	}
	var alignedSource []*image.NRGBA64
	if sourceMeta != nil {
		alignedSource, err = mediaio.DecodeRGBA64(sourceMeta.Path, mediaio.DecodeOptions{
			MaxFrames: options.MaxFrames, Width: width, Height: height,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("decode aligned source pixels: %w", err)
		}
	}

	for frameIndex, variantFrame := range decodedVariant {
		var sourceFrame *image.NRGBA64
		if frameIndex < len(alignedSource) {
			sourceFrame = alignedSource[frameIndex]
		}
		input := FrameInput{Index: frameIndex, Source: sourceFrame, Variant: variantFrame}
		if sourceMeta != nil && sourceMeta.FFProbe != nil {
			input.SourceAlphaMarked = mediameta.PixelFormatHasAlpha(sourceMeta.FFProbe.PixelFormat)
		}
		if meta.FFProbe != nil {
			input.VariantAlphaMarked = mediameta.PixelFormatHasAlpha(meta.FFProbe.PixelFormat)
		}
		analyzed := AnalyzeFrame(input)
		for issueIndex := range analyzed.Report.WrongParameters {
			analyzed.Report.WrongParameters[issueIndex].File = path
		}
		fileBase := fmt.Sprintf("variant-%d-%s", index, sanitizeBase(path))
		if err := WriteFrameArtifacts(options.OutputDir, fileBase, &analyzed, sourceFrame, variantFrame); err != nil {
			return nil, nil, err
		}
		wrong = append(wrong, analyzed.Report.WrongParameters...)
		variant.Frames = append(variant.Frames, analyzed.Report)
	}

	if len(variant.Frames) > 0 {
		stats := variant.Frames[0].VariantAlpha
		model := variant.Frames[0].VariantModel
		variant.File.AlphaStats = &stats
		variant.File.AlphaModel = model
	}
	return variant, wrong, nil
}

func sanitizeBase(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	replacer := strings.NewReplacer(" ", "_", "/", "_", ":", "_")
	return replacer.Replace(base)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func summarize(params []WrongParam) (string, string) {
	var errors, warnings, infos int
	for _, param := range params {
		switch param.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			infos++
		}
	}
	switch {
	case errors > 0:
		return "fail", fmt.Sprintf("发现 %d 个错误参数/通道冲突，%d 个警告，%d 个提示；修复帧已输出", errors, warnings, infos)
	case warnings > 0:
		return "warning", fmt.Sprintf("发现 %d 个警告，%d 个提示；未发现阻断性 Alpha 冲突", warnings, infos)
	default:
		return "pass", fmt.Sprintf("元数据、Alpha 标记和像素通道关系一致；%d 个提示", infos)
	}
}
