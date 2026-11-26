package main

import (
	"flag"
	"fmt"
	"os"

	"videopack/internal/bundle"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "videopack:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("videopack", flag.ContinueOnError)

	videoID := fs.String("video-id", "", "视频标识，写入清单（必填）")
	audioDir := fs.String("audio-dir", "", "音频源目录（必填）")
	subDir := fs.String("subtitle-dir", "", "字幕源目录（可选）")
	chapterDir := fs.String("chapter-dir", "", "章节源目录（可选）")
	coverDir := fs.String("cover-dir", "", "封面源目录（可选）")
	out := fs.String("out", "", "交付包输出目录（必填）")
	zipPath := fs.String("zip", "", "额外生成的 zip 归档路径（可选）")

	dup := fs.String("on-duplicate", "prefer", "重复语言策略: prefer|error")
	missing := fs.String("on-missing-subtitle", "warn", "缺失字幕策略: warn|fail")
	includeDup := fs.Bool("include-duplicates", false, "未入选的重复文件也复制进交付包")
	force := fs.Bool("force", false, "覆盖已存在的输出")
	meta := fs.String("metadata", "", "附加元数据，格式 key=value，可重复传入")

	if err := fs.Parse(args); err != nil {
		return err
	}

	metadata, err := parseMetadata(*meta, fs.Args())
	if err != nil {
		return err
	}
	opts := bundle.Options{
		VideoID:               *videoID,
		AudioDir:              *audioDir,
		SubtitleDir:           *subDir,
		ChapterDir:            *chapterDir,
		CoverDir:              *coverDir,
		Out:                   *out,
		ZipPath:               *zipPath,
		DuplicatePolicy:       bundle.DuplicatePolicy(*dup),
		MissingSubtitlePolicy: bundle.MissingSubtitlePolicy(*missing),
		IncludeDuplicates:     *includeDup,
		Force:                 *force,
		Metadata:              metadata,
	}

	result, err := bundle.Build(opts)
	if err != nil {
		return err
	}

	bundle.RenderWarnings(os.Stdout, result.Manifest.Warnings)
	bundle.RenderText(os.Stdout, result.Manifest.Matrix)
	fmt.Printf("package: %s\n", result.OutDir)
	fmt.Printf("files: %d\n", len(result.Manifest.Files))
	if result.ZipPath != "" {
		fmt.Printf("zip: %s\n", result.ZipPath)
	}
	return nil
}

// parseMetadata 支持 -metadata k=v 以及位置参数形式的 k=v。
func parseMetadata(inline string, rest []string) (map[string]string, error) {
	pairs := make([]string, 0, len(rest)+1)
	if inline != "" {
		pairs = append(pairs, inline)
	}
	pairs = append(pairs, rest...)

	var metadata map[string]string
	for _, pair := range pairs {
		k, v, ok := splitKV(pair)
		if !ok {
			return nil, fmt.Errorf("元数据必须是 key=value 格式: %q", pair)
		}
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata[k] = v
	}
	return metadata, nil
}

func splitKV(pair string) (string, string, bool) {
	for i := range pair {
		if pair[i] == '=' {
			key := pair[:i]
			if key == "" {
				return "", "", false
			}
			return key, pair[i+1:], true
		}
	}
	return "", "", false
}
