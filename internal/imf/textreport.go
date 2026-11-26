package imf

import (
	"fmt"
	"strings"
)

// RenderTextReport produces a human-readable Chinese report covering the
// final track list plus missing/duplicate assets.
func RenderTextReport(rep *Report, pl *Playlist) string {
	var b strings.Builder
	b.WriteString("IMF 合并播放清单 — 资产核对报告\n")
	b.WriteString("================================\n\n")

	if pl != nil {
		fmt.Fprintf(&b, "最终 CPL ID : %s\n", URN(pl.OutputCPLID))
		fmt.Fprintf(&b, "基础 CPL    : %s\n", URN(pl.BaseCPL))
		if pl.SelectedOPL != nil {
			fmt.Fprintf(&b, "引用 OPL    : %s -> %v\n", URN(pl.SelectedOPL.ID), urnList(pl.SelectedOPL.CPLRefs))
		}
		fmt.Fprintf(&b, "来源 CPL    : %d 个\n", len(pl.SourceCPLs))
		for _, c := range pl.SourceCPLs {
			fmt.Fprintf(&b, "  - %s [%s] %s\n", URN(c.ID), c.PackageLabel, c.Annotation)
		}
		fmt.Fprintf(&b, "轨道/资源   : %d 条轨道, %d 个资源; 可解析 %d, 缺失 %d\n\n",
			pl.TrackCount, pl.ResourceCount, pl.ResolvedCount, pl.MissingCount)

		b.WriteString("最终轨道清单:\n")
		for si, seg := range pl.Segments {
			fmt.Fprintf(&b, "  Segment %d %s\n", si+1, URN(seg.ID))
			for _, tr := range seg.Tracks {
				fmt.Fprintf(&b, "    [%s] %s (TrackId=%s) 资源 %d 个  来源:%s\n",
					trackKindCN(tr.Kind), tr.SequenceName, URN(tr.TrackID),
					len(tr.Resources), strings.Join(tr.Provenance, ","))
				for _, r := range tr.Resources {
					state := "未知"
					path := ""
					if r.Resolved != nil {
						switch {
						case r.Resolved.Found && r.Resolved.FileExists:
							state = "已解析"
						case r.Resolved.Found:
							state = "文件缺失"
						default:
							state = "引用丢失"
						}
						path = r.Resolved.AbsolutePath
					}
					fmt.Fprintf(&b, "        - %s -> %s [%s] %s\n",
						shortID(r.ID), URN(r.TrackFileID), state, path)
				}
			}
		}
		b.WriteString("\n")
	}

	s := rep.Summary
	fmt.Fprintf(&b, "汇总: 缺失 %d (悬空引用 %d / 文件缺失 %d), 重复 %d (一致 %d / 冲突 %d), 警告 %d\n\n",
		s.MissingTotal, s.UnresolvedRefs, s.MissingFiles,
		s.DuplicateTotal, s.DuplicateIdentical, s.DuplicateConflicts, len(rep.Warnings))

	b.WriteString("【缺失资产 / 断裂引用】\n")
	if len(rep.Missing) == 0 {
		b.WriteString("  (无)\n")
	}
	for _, m := range rep.Missing {
		fmt.Fprintf(&b, "  ✗ [%s] %s\n", m.Kind, m.Message)
		if m.ReferencedBy != "" {
			fmt.Fprintf(&b, "      引用方: %s\n", m.ReferencedBy)
		}
		if m.Path != "" {
			fmt.Fprintf(&b, "      路径  : %s\n", m.Path)
		}
	}
	b.WriteString("\n")

	b.WriteString("【重复资产】\n")
	if len(rep.Duplicates) == 0 {
		b.WriteString("  (无)\n")
	}
	for _, d := range rep.Duplicates {
		icon := "≈"
		if d.Severity == SeverityConflict {
			icon = "!"
		}
		fmt.Fprintf(&b, "  %s [%s/%s] %s\n", icon, d.Kind, d.Severity, d.Message)
		for _, o := range d.Occurrences {
			fmt.Fprintf(&b, "      - 包 %q AM=%v PKL=%v exists=%v hash=%s size=%d\n",
				o.PackageLabel, o.InAssetMap, o.InPKL, o.FileExists, o.Hash, o.Size)
		}
	}
	b.WriteString("\n")

	if len(rep.Warnings) > 0 {
		b.WriteString("【警告】\n")
		for _, wn := range rep.Warnings {
			fmt.Fprintf(&b, "  ! [%s] %s\n", wn.Kind, wn.Message)
		}
	}
	return b.String()
}

func trackKindCN(kind string) string {
	switch kind {
	case "image":
		return "画面"
	case "audio":
		return "声音"
	case "subtitle":
		return "字幕"
	case "data":
		return "数据"
	default:
		return "其他"
	}
}

func urnList(ids []string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = URN(id)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
