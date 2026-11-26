package bundle

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Undetermined 是无法从文件名解析出语言时使用的兜底标签。
const Undetermined = "und"

var langPattern = regexp.MustCompile(`(?:^|-)([a-z]{2,3}(?:-[a-z0-9]{2,4})?)(?:-|$)`)

// nonLanguageTags 收集常出现在文件名中但不是语言代码的短词。
var nonLanguageTags = map[string]bool{
	"alt": true, "raw": true, "new": true, "old": true,
	"hq": true, "lq": true, "v2": true, "v3": true,
}

// extensions 记录每种轨道类型接受的文件后缀及其编码标签。
var extensions = map[Kind]map[string]string{
	KindAudio: {
		".m4a": "aac", ".aac": "aac", ".mp3": "mp3", ".ac3": "ac3",
		".eac3": "eac3", ".flac": "flac", ".opus": "opus",
		".ogg": "vorbis", ".wav": "pcm",
	},
	KindSubtitle: {
		".srt": "srt", ".vtt": "webvtt", ".ass": "ass",
		".ssa": "ssa", ".ttml": "ttml", ".xml": "ttml",
	},
	KindChapter: {
		".xml": "xml", ".txt": "txt", ".csv": "csv", ".json": "json",
	},
	KindCover: {
		".jpg": "jpeg", ".jpeg": "jpeg", ".png": "png",
		".webp": "webp", ".avif": "avif",
	},
}

// LanguageFromName 从文件名中提取 BCP-47 风格的语言标签。
// 例如 "movie.en-US.ass" -> "en-US"；无法识别时返回 und。
func LanguageFromName(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	normalized := strings.ToLower(base)
	for _, sep := range []string{"_", "."} {
		normalized = strings.ReplaceAll(normalized, sep, "-")
	}
	normalized = regexp.MustCompile(`-+`).ReplaceAllString(normalized, "-")
	matches := langPattern.FindAllStringSubmatch(normalized, -1)
	var tags []string
	for _, m := range matches {
		if nonLanguageTags[m[1]] {
			continue
		}
		parts := strings.Split(m[1], "-")
		if len(parts) > 1 && nonLanguageTags[parts[1]] {
			m[1] = parts[0]
		}
		tags = append(tags, m[1])
	}
	if len(tags) == 0 {
		return Undetermined
	}
	return normalizeTag(tags[len(tags)-1])
}

func normalizeTag(tag string) string {
	parts := strings.Split(tag, "-")
	parts[0] = strings.ToLower(parts[0])
	for i := 1; i < len(parts); i++ {
		p := strings.ToLower(parts[i])
		if len(p) == 2 {
			parts[i] = strings.ToUpper(p)
		} else if len(p) > 2 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "-")
}

func codecFor(k Kind, name string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	codec, ok := extensions[k][ext]
	return codec, ok
}

// scanDir 递归扫描单个轨道目录，返回按路径排序的候选轨道。
func scanDir(k Kind, dir string) ([]Track, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("%s 目录解析失败: %w", k, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("%s 目录不可用: %w", k, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s 路径不是目录: %s", k, abs)
	}

	var tracks []Track
	walkErr := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		codec, ok := codecFor(k, p)
		if !ok {
			return nil
		}
		finfo, err := os.Stat(p)
		if err != nil {
			return err
		}
		lang := LanguageFromName(p)
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return err
		}
		tracks = append(tracks, Track{
			Kind:      k,
			Language:  lang,
			SourceAbs: p,
			RelPath:   portableRelPath(k, lang, rel),
			Codec:     codec,
			Size:      finfo.Size(),
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("扫描 %s 目录失败: %w", k, walkErr)
	}
	slices.SortFunc(tracks, func(a, b Track) int {
		return strings.Compare(a.SourceAbs, b.SourceAbs)
	})
	ensureUniquePaths(tracks)
	return tracks, nil
}

// ensureUniquePaths 解决不同子目录下同名文件落到同一目标路径的冲突。
func ensureUniquePaths(tracks []Track) {
	seen := map[string]int{}
	for i := range tracks {
		rel := tracks[i].RelPath
		n, exists := seen[rel]
		if !exists {
			seen[rel] = 1
			continue
		}
		seen[rel] = n + 1
		ext := path.Ext(rel)
		tracks[i].RelPath = strings.TrimSuffix(rel, ext) +
			fmt.Sprintf("~%d", n+1) + ext
	}
}

// portableRelPath 生成交付包内的相对路径，统一使用正斜杠。
func portableRelPath(k Kind, lang, sourceRel string) string {
	base := filepath.Base(sourceRel)
	return path.Join(string(k), lang, base)
}

// scanAll 扫描 Options 中配置的全部目录。
func scanAll(opts Options) (map[Kind][]Track, error) {
	found := make(map[Kind][]Track, len(AllKinds))
	for _, k := range AllKinds {
		tracks, err := scanDir(k, opts.dirFor(k))
		if err != nil {
			return nil, err
		}
		found[k] = tracks
	}
	if len(found[KindAudio]) == 0 {
		return nil, fmt.Errorf("音频目录中没有可用的音频文件: %q", opts.AudioDir)
	}
	return found, nil
}
