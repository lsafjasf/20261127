package bundle

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// codecRank 为同语言的多个候选提供确定性的优先级，数值越大越优先。
var codecRank = map[Kind]map[string]int{
	KindAudio: {
		"flac": 8, "eac3": 7, "ac3": 6, "opus": 5,
		"aac": 4, "vorbis": 3, "mp3": 2, "pcm": 1,
	},
	KindSubtitle: {"ass": 6, "ssa": 5, "webvtt": 4, "ttml": 3, "srt": 2},
	KindChapter:  {"json": 5, "xml": 4, "csv": 3, "txt": 2},
	KindCover:    {"png": 5, "avif": 4, "webp": 3, "jpeg": 2},
}

// buildMatrix 汇总候选轨道，完成重复语言裁决与缺失字幕检查。
func buildMatrix(found map[Kind][]Track, opts Options) (Matrix, []string, error) {
	warnings := []string{}

	for _, k := range AllKinds {
		byLang := map[string][]Track{}
		for _, t := range found[k] {
			byLang[t.Language] = append(byLang[t.Language], t)
		}
		for lang, group := range byLang {
			if len(group) <= 1 {
				continue
			}
			if opts.DuplicatePolicy == DuplicateError {
				return Matrix{}, nil, fmt.Errorf("%s 存在重复语言 %q（%d 个候选）", k, lang, len(group))
			}
			slices.SortFunc(group, compareTrack)
			group[0].Chosen = true
			for i := 1; i < len(group); i++ {
				group[i].Duplicate = true
				warnings = append(warnings, fmt.Sprintf(
					"%s 语言 %q 存在重复，已选用 %s，跳过 %s",
					k, lang, group[0].RelPath, group[i].RelPath))
			}
			byLang[lang] = group
		}
		merged := []Track{}
		keys := make([]string, 0, len(byLang))
		for lang := range byLang {
			keys = append(keys, lang)
		}
		slices.Sort(keys)
		for _, lang := range keys {
			merged = append(merged, byLang[lang]...)
		}
		found[k] = merged
	}

	langSet := map[string]struct{}{}
	for _, tracks := range found {
		for _, t := range tracks {
			langSet[t.Language] = struct{}{}
		}
	}
	languages := make([]string, 0, len(langSet))
	for lang := range langSet {
		languages = append(languages, lang)
	}
	slices.Sort(languages)

	subLangs := map[string]bool{}
	for _, t := range found[KindSubtitle] {
		subLangs[t.Language] = true
	}
	audioLangs := map[string]bool{}
	for _, t := range found[KindAudio] {
		audioLangs[t.Language] = true
	}
	for lang := range audioLangs {
		if subLangs[lang] {
			continue
		}
		msg := fmt.Sprintf("音频语言 %q 缺少对应字幕", lang)
		if opts.MissingSubtitlePolicy == MissingSubtitleFail {
			return Matrix{}, nil, errors.New(msg)
		}
		warnings = append(warnings, msg)
	}

	cells := make(map[Kind]map[string]*Cell, len(AllKinds))
	for _, k := range AllKinds {
		cells[k] = map[string]*Cell{}
		byLang := map[string][]Track{}
		for _, t := range found[k] {
			byLang[t.Language] = append(byLang[t.Language], t)
		}
		for _, lang := range languages {
			group := byLang[lang]
			cell := &Cell{Present: len(group) > 0, Tracks: group}
			for _, t := range group {
				if t.Chosen {
					cell.Chosen = t.RelPath
				}
			}
			cells[k][lang] = cell
		}
	}

	return Matrix{Languages: languages, Cells: cells}, warnings, nil
}

// compareTrack 提供确定性排序：编码优先级 > 文件大小 > 路径字典序。
func compareTrack(a, b Track) int {
	ra := codecRank[a.Kind][a.Codec]
	rb := codecRank[b.Kind][b.Codec]
	if ra != rb {
		if ra > rb {
			return -1
		}
		return 1
	}
	if a.Size != b.Size {
		if a.Size > b.Size {
			return -1
		}
		return 1
	}
	return strings.Compare(a.SourceAbs, b.SourceAbs)
}

// selectedTracks 返回需要写入交付包的轨道（默认排除未入选的重复项）。
func selectedTracks(found map[Kind][]Track, includeDuplicates bool) []Track {
	var out []Track
	for _, k := range AllKinds {
		for _, t := range found[k] {
			if t.Duplicate && !includeDuplicates {
				continue
			}
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b Track) int {
		if a.Kind != b.Kind {
			return strings.Compare(string(a.Kind), string(b.Kind))
		}
		return strings.Compare(a.RelPath, b.RelPath)
	})
	return out
}
