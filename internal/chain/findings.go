package chain

import (
	"fmt"

	"evidencechain/internal/model"
)

func (v *verifier) buildFindings() {
	f := &v.res.Findings
	allIntact := true
	for _, a := range v.res.Anchors {
		if a.Status != "intact" {
			allIntact = false
		}
	}
	if allIntact {
		*f = append(*f, model.Finding{
			ID: "F-SEAL", Title: "原始材料封印完好", Severity: "info", Status: "confirmed",
			Summary: fmt.Sprintf("%d 件原始材料的 SHA-256 与哈希索引一致，信任基准未被改动。", len(v.res.Anchors)),
		})
	}

	var renamed []string
	for _, o := range v.res.Files {
		if o.Status == stIdentified && o.RenamedFrom != "" {
			renamed = append(renamed, fmt.Sprintf("%s（原名 %s → 现名 %s）", o.MatchedEvidence, o.RenamedFrom, o.Path))
		}
	}
	if len(renamed) > 0 {
		*f = append(*f, model.Finding{
			ID: "F-RENAME", Title: "改名文件经内容哈希同一认定", Severity: "confirmed", Status: "confirmed",
			Summary: fmt.Sprintf("%d 个文件改名但完整 SHA-256 精确命中原件：%s", len(renamed), joinLines(renamed)),
		})
	}

	for _, c := range v.res.Containers {
		if c.Status == "pristine-anchor-set" {
			*f = append(*f, model.Finding{
				ID: "F-REPACK-" + baseID(c.Path), Title: "容器改名/重排但成员集合未变", Severity: "confirmed", Status: "confirmed",
				Summary:  fmt.Sprintf("%s 的成员内容集合摘要与全部基准原件一致；条目改名、顺序重排和重打包只改变容器文件哈希。", c.Path),
				Evidence: []string{c.Path, c.MemberSetSHA256},
			})
		}
	}

	nTamper := 0
	for _, o := range v.res.Files {
		if o.Status == stTampered {
			nTamper++
			*f = append(*f, model.Finding{
				ID: "F-TAMPER-" + baseID(o.Path), Title: "检测到局部篡改并定位块范围", Severity: "critical", Status: "confirmed",
				Summary:  fmt.Sprintf("%s 相对候选基准 %s：%d/%d 个对齐块完全匹配，%d 个块不一致，篡改范围已定位到字节区间。", o.Path, o.Candidate, o.MatchedBlocks, o.TotalBlocks, len(o.BlockDiffs)),
				Evidence: []string{o.Path, o.Candidate},
			})
		}
	}
	for _, c := range v.res.Containers {
		for _, m := range c.Members {
			if m.Status == stTampered {
				nTamper++
				*f = append(*f, model.Finding{
					ID: "F-TAMPER-M-" + baseID(m.Name), Title: "容器成员被局部篡改", Severity: "critical", Status: "confirmed",
					Summary:  fmt.Sprintf("%s#%s 相对 %s 出现块级差异。", c.Path, m.Name, m.Candidate),
					Evidence: []string{c.Path + "#" + m.Name, m.Candidate},
				})
			}
		}
	}
	_ = nTamper

	for _, o := range v.res.Files {
		if o.Status == stSimilarOnly {
			*f = append(*f, model.Finding{
				ID: "F-SIM-" + baseID(o.Path), Title: "高相似但不能认定同源", Severity: "warning", Status: "uncertain",
				Summary: fmt.Sprintf("%s 与 %s 的 Jaccard 相似度 %.1f%%、SimHash 距离 %d；仅为相似性线索，来源关系不确定。", o.Path, o.Candidate, o.Fuzzy.JaccardPercent, o.Fuzzy.HammingBits),
			})
		}
		if o.Status == stUnknown {
			*f = append(*f, model.Finding{
				ID: "F-UNK-" + baseID(o.Path), Title: "未知检材", Severity: "warning", Status: "uncertain",
				Summary: fmt.Sprintf("%s 无哈希命中且相似性不足，来源与用途无法从哈希证据确定。", o.Path),
			})
		}
	}

	for _, w := range v.res.Watchlist {
		if w.Status == "exact-hit" {
			*f = append(*f, model.Finding{
				ID: "F-WL-HIT-" + baseID(w.Label), Title: "协查哈希精确命中", Severity: "confirmed", Status: "confirmed",
				Summary: fmt.Sprintf("%s 完整哈希命中 %s。", w.Label, w.Against),
			})
		}
		if w.Status == "rejected-prefix-only" {
			*f = append(*f, model.Finding{
				ID: "F-WL-DECOY-" + baseID(w.Label), Title: "相似哈希诱饵被拒绝同一认定", Severity: "info", Status: "confirmed",
				Summary: w.Label + "：" + w.Detail,
			})
		}
	}
}

func joinLines(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += "；"
		}
		out += x
	}
	return out
}

func baseID(p string) string {
	// short stable-ish suffix for finding IDs
	b := []byte(p)
	h := 0
	for _, c := range b {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return fmt.Sprintf("%x", h%0xffff)
}

func (v *verifier) finalizeSummary() {
	s := v.res.Summary
	s["anchors"] = len(v.res.Anchors)
	for _, a := range v.res.Anchors {
		s["anchor_"+a.Status]++
	}
	s["field_files"] = len(v.res.Files)
	for _, f := range v.res.Files {
		s["file_"+f.Status]++
	}
	s["containers"] = len(v.res.Containers)
	for _, c := range v.res.Containers {
		s["container_"+c.Status]++
	}
	s["provenance_links"] = len(v.res.Links)
	for _, l := range v.res.Links {
		s["link_"+l.Status]++
	}
	s["watchlist_items"] = len(v.res.Watchlist)
	for _, w := range v.res.Watchlist {
		s["watch_"+w.Status]++
	}
	s["judgments"] = len(v.res.Judgments)
	for _, j := range v.res.Judgments {
		s["judgment_"+j.Status]++
	}
	s["findings"] = len(v.res.Findings)
	s["errors"] = len(v.res.Errors)
	s["warnings"] = len(v.res.Warnings)
}
