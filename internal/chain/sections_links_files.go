package chain

import (
	"fmt"
)

func linkStatusLabel(s string) string {
	switch s {
	case "verified":
		return "✅ 已验证"
	case "broken":
		return "❌ 断裂"
	case "unsupported":
		return "⚠️ 未知关系"
	}
	return s
}

func fileStatusLabel(s string) string {
	switch s {
	case stIdentified:
		return "✅ 已同一认定"
	case stTampered:
		return "🛠 局部篡改"
	case stSimilarOnly:
		return "🟡 仅相似/不确定"
	case stUnknown:
		return "❔ 未知"
	}
	return s
}

func (r *reportRenderer) sectionLinks() {
	r.ln("## 五、派生副本与保管链验证")
	r.ln("")
	if len(r.v.Links) == 0 {
		r.ln("未提供 `custody/provenance.json`。")
		r.ln("")
		return
	}
	r.ln("| 链号 | 派生目标 | 关系 | 审计 | 说明 |")
	r.ln("|---|---|---|---|---|")
	for _, l := range r.v.Links {
		r.p("| `%s` | `%s` | `%s` | %s | %s |\n",
			l.ID, mdCell(l.To), l.Relation, linkStatusLabel(l.Status), mdCell(l.Detail))
	}
	r.ln("")
	r.ln("`byte-copy` 复算 SHA-256；`normalize-eol` 用同一确定性函数复算文本；`png-reencode` 解码后逐像素比较；`zip-pack/tar-pack` 比较成员集合摘要。全部 `verified` 表示派生层可从原始材料无损/可复现地得到。")
	r.ln("")
}

func (r *reportRenderer) sectionFiles() {
	r.ln("## 六、改名文件、局部篡改与未知检材")
	r.ln("")
	r.ln("| 回传文件 | 尺寸 | SHA-256(前16) | 判定 | 命中/候选 | 相似度(J/SimHash距离) | 对齐块 |")
	r.ln("|---|---:|---|---|---|---|---|")
	for _, f := range r.v.Files {
		who := f.MatchedEvidence
		if who == "" && f.Candidate != "" {
			who = "候选 " + f.Candidate
		}
		sim := "—"
		if f.Fuzzy != nil {
			lead := ""
			if f.Fuzzy.Lead {
				lead = "（达线索阈值）"
			}
			sim = fmt.Sprintf("%.1f%% / %d bit%s", f.Fuzzy.JaccardPercent, f.Fuzzy.HammingBits, lead)
		}
		blocks := "—"
		if f.TotalBlocks > 0 {
			blocks = fmt.Sprintf("匹配 %d/%d，差异块 %d", f.MatchedBlocks, f.TotalBlocks, len(f.BlockDiffs))
		}
		r.p("| %s | %d | `%s` | %s | %s | %s | %s |\n",
			r.fileLink(f.Path), f.Size, shortHash(f.SHA256, 16), fileStatusLabel(f.Status), codeOrDash(who), sim, blocks)
	}
	r.ln("")

	for _, f := range r.v.Files {
		if f.Status != stTampered || len(f.BlockDiffs) == 0 {
			continue
		}
		r.p("**篡改定位：%s（候选基准 `%s`，%d/%d 块匹配）**\n\n",
			r.fileLink(f.Path), f.Candidate, f.MatchedBlocks, f.TotalBlocks)
		r.ln("以下仅列出不一致块；其余对齐块 SHA-256 完全相同。")
		r.ln("")
		r.ln("| 块号 | 字节起 | 字节止(不含) | 状态 |")
		r.ln("|---:|---:|---:|---|")
		for _, d := range f.BlockDiffs {
			r.p("| %d | %d | %d | ❌ %s |\n", d.ChunkIndex, d.Start, d.End, diffLabel(d.Status))
		}
		r.ln("")
	}

	r.ln("> 判读要点：精确哈希命中且现名不同于原件名即“改名”；多数对齐块匹配且存在差异块才判“局部篡改”；")
	r.ln("> 仅有相似度信号则降级为“仅相似”，并由分析员追加 `uncertain-origin` 判读。")
	r.ln("")
}

func diffLabel(s string) string {
	switch s {
	case "differ":
		return "块内容不同"
	case "extra":
		return "questioned 多出的块"
	case "missing":
		return "questioned 缺失的块"
	}
	return s
}
