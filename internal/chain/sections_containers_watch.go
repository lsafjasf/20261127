package chain

import (
	"fmt"
	"strings"
)

func containerStatusLabel(s string) string {
	switch s {
	case "pristine-anchor-set":
		return "✅ 成员集合=全部基准（改名/重排）"
	case "pristine-subset":
		return "✅ 基准子集（跨容器迁移）"
	case "contains-altered-or-unknown":
		return "🛠 含篡改/未知成员"
	case "unreadable":
		return "❌ 无法解析"
	}
	return s
}

func watchStatusLabel(s string) string {
	switch s {
	case "exact-hit":
		return "✅ 完整哈希精确命中"
	case "rejected-prefix-only":
		return "⛔ 仅前缀相似，拒绝同一认定"
	case "similarity-lead":
		return "🟡 仅相似线索（非同一）"
	case "no-match":
		return "⚪ 无命中"
	case "malformed":
		return "⛔ 哈希残缺/无效"
	}
	return s
}

func (r *reportRenderer) sectionContainers() {
	r.ln("## 七、容器重排与跨容器迁移")
	r.ln("")
	r.ln("| 容器 | 格式 | 容器文件SHA(前12) | 成员集合摘要(前16) | 状态 |")
	r.ln("|---|---|---|---|---|")
	for _, c := range r.v.Containers {
		r.p("| %s | `%s` | `%s` | `%s` | %s |\n",
			r.fileLink(c.Path), c.Format, shortHash(c.FileSHA256, 12),
			shortHash(c.MemberSetSHA256, 16), containerStatusLabel(c.Status))
	}
	r.ln("")
	for _, c := range r.v.Containers {
		r.p("**成员明细：`%s`**\n\n", c.Path)
		r.ln("| 成员名 | 尺寸 | SHA-256(前12) | 判定 | 命中/候选 | 改名自 | 相似度 |")
		r.ln("|---|---:|---|---|---|---|---|")
		for _, m := range c.Members {
			who := m.MatchedEvidence
			if who == "" && m.Candidate != "" {
				who = "候选 " + m.Candidate
			}
			sim := "—"
			if m.Fuzzy != nil {
				sim = fmt.Sprintf("%.1f%%/%dbit", m.Fuzzy.JaccardPercent, m.Fuzzy.HammingBits)
			}
			ren := m.RenamedFrom
			if ren == "" {
				ren = "—"
			}
			r.p("| `%s` | %d | `%s` | %s | %s | %s | %s |\n",
				mdCell(m.Name), m.Size, shortHash(m.SHA256, 12),
				fileStatusLabel(m.Status), codeOrDash(who), mdCell(ren), sim)
		}
		r.ln("")
		if c.Status == "pristine-anchor-set" {
			r.ln("> 该容器的 `memberset_sha256` 与四件原件组成的集合摘要逐位相同；容器整包哈希不同只能说明重打包/元数据/顺序变化。")
			r.ln("")
		}
	}
}

func (r *reportRenderer) sectionWatch() {
	r.ln("## 八、协查哈希审计：相似哈希不是同一证据")
	r.ln("")
	if len(r.v.Watchlist) == 0 {
		r.ln("未提供 `watchlist/reference-hashes.json`。")
		r.ln("")
		return
	}
	r.ln("| 协查项 | 类型 | 提交值(前18) | 审计结论 | 对应对象 | 说明 |")
	r.ln("|---|---|---|---|---|---|")
	for _, w := range r.v.Watchlist {
		against := strings.ReplaceAll(mdCell(w.Against), " ; ", "<br>")
		if against == "" {
			against = "—"
		}
		r.p("| %s | `%s` | `%s` | %s | %s | %s |\n",
			mdCell(w.Label), w.Kind, shortHash(w.Value, 18),
			watchStatusLabel(w.Status), against, mdCell(w.Detail))
	}
	r.ln("")
	r.ln("**关键防线**：WL-2 与 EV-003 共享 12 位十六进制前缀（48 bit），但剩余 52 位完全不同。")
	r.ln("前缀相同可能来自截断、转录笔误，也可能是人为构造；任何自动化系统都不得把它报告为“同一来源”。")
	r.ln("WL-3 的 SimHash 与 EV-001 汉明距离为 0/64，但 SimHash 相同只代表特征分布高度接近，")
	r.ln("被篡改后的日志同样可能距离很小甚至为 0；同一认定只接受完整 SHA-256 精确相等。")
	r.ln("")
}
