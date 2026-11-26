package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const manifestName = "manifest.json"

// Result 描述一次成功打包的产物。
type Result struct {
	OutDir   string
	ZipPath  string
	Manifest Manifest
}

// Build 执行完整的扫描、矩阵构建与原子打包。
// 任何阶段失败都会清理暂存目录与半成品，保证目标路径无残留。
func Build(opts Options) (result *Result, err error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	parent := filepath.Dir(opts.Out)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出父目录失败: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".videopack-stage-*")
	if err != nil {
		return nil, fmt.Errorf("创建暂存目录失败: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()

	found, err := scanAll(opts)
	if err != nil {
		return nil, err
	}
	matrix, warnings, err := buildMatrix(found, opts)
	if err != nil {
		return nil, err
	}

	tracks := selectedTracks(found, opts.IncludeDuplicates)
	files := make([]FileEntry, 0, len(tracks))
	for _, t := range tracks {
		entry, err := copyTrack(stage, t)
		if err != nil {
			return nil, err
		}
		files = append(files, entry)
	}

	manifest := Manifest{
		Schema:    "videopack/v1",
		VideoID:   opts.VideoID,
		CreatedAt: clockNow(),
		Matrix:    matrix,
		Files:     files,
		Warnings:  warnings,
		Metadata:  opts.Metadata,
	}
	if err := writeManifest(stage, manifest); err != nil {
		return nil, err
	}

	var zipPath string
	if opts.ZipPath != "" {
		zipPath, err = writeZip(opts.ZipPath, stage, opts.Force)
		if err != nil {
			return nil, err
		}
	}

	if err := commitStage(stage, opts.Out, opts.Force); err != nil {
		return nil, err
	}
	committed = true
	return &Result{OutDir: opts.Out, ZipPath: zipPath, Manifest: manifest}, nil
}

func (o *Options) validate() error {
	if strings.TrimSpace(o.VideoID) == "" {
		return fmt.Errorf("video-id 不能为空")
	}
	if strings.TrimSpace(o.AudioDir) == "" {
		return fmt.Errorf("audio-dir 不能为空")
	}
	if strings.TrimSpace(o.Out) == "" {
		return fmt.Errorf("out 不能为空")
	}
	if o.DuplicatePolicy == "" {
		o.DuplicatePolicy = DuplicatePrefer
	}
	if o.MissingSubtitlePolicy == "" {
		o.MissingSubtitlePolicy = MissingSubtitleWarn
	}
	if o.DuplicatePolicy != DuplicatePrefer && o.DuplicatePolicy != DuplicateError {
		return fmt.Errorf("未知重复语言策略: %q", o.DuplicatePolicy)
	}
	if o.MissingSubtitlePolicy != MissingSubtitleWarn && o.MissingSubtitlePolicy != MissingSubtitleFail {
		return fmt.Errorf("未知缺失字幕策略: %q", o.MissingSubtitlePolicy)
	}
	if _, err := filepath.Abs(o.Out); err != nil {
		return fmt.Errorf("输出路径非法: %w", err)
	}
	if _, err := os.Stat(o.Out); err == nil && !o.Force {
		return fmt.Errorf("输出路径已存在（使用 --force 覆盖）: %s", o.Out)
	}
	return nil
}

// commitStage 原子地用暂存目录替换最终目录。
func commitStage(stage, out string, force bool) error {
	if err := syncDir(filepath.Dir(stage)); err != nil {
		return err
	}
	if force {
		if _, err := os.Stat(out); err == nil {
			if err := os.RemoveAll(out); err != nil {
				return fmt.Errorf("清理旧输出失败: %w", err)
			}
		}
	}
	if err := os.Rename(stage, out); err != nil {
		return fmt.Errorf("提交交付包失败: %w", err)
	}
	return syncDir(filepath.Dir(out))
}

func writeManifest(stage string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化清单失败: %w", err)
	}
	data = append(data, '\n')
	dst := filepath.Join(stage, manifestName)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("写入清单失败: %w", err)
	}
	return syncDir(stage)
}
