package chain

import (
	"path/filepath"

	"evidencechain/internal/digestx"
)

func verdictLabel(v string) string {
	m := map[string]string{
		"identity-confirmed":        "同一认定",
		"rename-confirmed":          "改名同一认定",
		"tamper-localized":          "局部篡改定位",
		"container-rearranged":      "容器改名/重排",
		"container-set-confirmed":   "容器集合确认",
		"container-contains-tamper": "容器含篡改成员",
		"similarity-only":           "仅主张相似",
		"uncertain-origin":          "来源不确定（保留）",
		"unknown-item":              "未知检材",
		"watchlist-hit":             "协查精确命中",
		"watchlist-rejected":        "拒绝伪命中",
		"watchlist-similarity-lead": "协查相似线索",
		"link-verified":             "保管链已验证",
	}
	if z, ok := m[v]; ok {
		return z
	}
	return v
}

func judgeAuditLabel(s string) string {
	switch s {
	case "supported":
		return "✅ 与事实一致"
	case "uncertain-allowed":
		return "🟡 不确定性已显式保留"
	case "contradiction":
		return "❌ 与事实矛盾"
	case "overclaim":
		return "⛔ 过度断言（相似≠同一）"
	case "unknown-target":
		return "❌ 目标不存在"
	}
	return s
}

func (r *reportRenderer) sectionNotes() {
	if r.notes == "" {
		return
	}
	// The notes file already carries its own "## 九、" heading.
	r.ln(r.notes)
	r.ln("")
}

func (r *reportRenderer) sectionJudgments() {
	r.ln("## 十、人工判读记录及一致性审计")
	r.ln("")
	if r.js == nil || len(r.js.Items) == 0 {
		r.ln("缺少 `review/judgments.json`，证据链不完整。")
		r.ln("")
		return
	}
	r.p("分析员：%s；复核时间：`%s`。\n\n", mdCell(r.js.Analyst), r.js.ReviewedAt)
	audit := map[string]string{}
	for _, j := range r.v.Judgments {
		audit[j.ID] = j.Status
	}
	r.ln("| 编号 | 对象 | 结论 | 置信度 | 机器审计 | 依据 | 理由摘要 |")
	r.ln("|---|---|---|---|---|---|---|")
	for _, j := range r.js.Items {
		r.p("| `%s` | `%s` | %s | %s | %s | `%s` | %s |\n",
			j.ID, mdCell(j.Target), verdictLabel(j.Verdict), mdCell(j.Confidence),
			judgeAuditLabel(audit[j.ID]), mdCell(joinBasis(j.Basis)), mdCell(j.Rationale))
	}
	r.ln("")
	r.ln("审计规则：同一/命中类结论必须存在完整 SHA-256 精确命中事实；篡改结论必须存在块级差异；")
	r.ln("仅凭模糊/前缀哈希主张同源会得到 `overclaim` 并使 `verify` 以退出码 3 失败。")
	r.ln("")
}

func joinBasis(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

func (r *reportRenderer) sectionRepro() {
	r.ln("## 十一、可复现性与证据链文件")
	r.ln("")
	r.ln("全部 Go 代码仅使用标准库，可在断网环境完整构建：")
	r.ln("")
	r.ln("```bash")
	r.ln("GOPROXY=off CGO_ENABLED=0 go build ./...")
	r.ln("GOPROXY=off go test ./...")
	r.ln("go run ./cmd/evidencechain verify --case case --write-json case/out/verification.json")
	r.ln("go run ./cmd/evidencechain report --case case --out FORENSIC-REPORT.md")
	r.ln("```")
	r.ln("")
	r.ln("从零重建参考案例（不会覆盖 `case/review/` 下的人工判读）：")
	r.ln("")
	r.ln("```bash")
	r.ln("bash scripts/rebuild-case.sh")
	r.ln("```")
	r.ln("")
	r.ln("关键记录文件：")
	r.ln("")
	r.ln("| 文件 | 内容 |")
	r.ln("|---|---|")
	r.ln("| `case/index/hash-index.json` | 原件全量/块/模糊哈希索引 |")
	r.ln("| `case/custody/provenance.json` | 派生与保管关系图 |")
	r.ln("| `case/review/judgments.json` | 人工判读（机器审计输入） |")
	r.ln("| `case/review/analyst-notes.md` | 手工勘验笔记原文 |")
	r.ln("| `case/watchlist/reference-hashes.json` | 外部协查哈希（含前缀诱饵） |")
	r.ln("| `case/out/verification.json` | 结构化验证结果 |")
	r.ln("")

	r.ln("**索引与判读文件自身的 SHA-256（防篡改锚）**")
	r.ln("")
	for _, rel := range []string{IndexRel, "review/judgments.json", "custody/provenance.json", "watchlist/reference-hashes.json"} {
		full := filepath.Join(r_caseDir(r), filepath.FromSlash(rel))
		if h, _, err := digestx.SHA256File(full); err == nil {
			r.p("- `case/%s` → `%s`\n", rel, h)
		}
	}
	r.ln("")
	r.ln("> 这些哈希只能证明报告生成时上述记录未被意外改动；若要形成法律意义上的保管链，")
	r.ln("> 还需由第三方/时间戳服务或代码提交记录对这些哈希再固定。")
	r.ln("")

	r.ln("## 附录 A：完整哈希值")
	r.ln("")
	r.ln("**A.1 原始材料完整 SHA-256 与块根**")
	r.ln("")
	for _, e := range r.idx.Entries {
		r.p("- `%s` (%s)\n  - SHA-256: `%s`\n  - chunk_root: `%s`\n  - simhash: `%s`\n",
			e.EvidenceID, e.LogicalName, e.SHA256, e.ChunkRoot, e.Fuzzy.SimHash)
	}
	r.ln("")
	r.ln("**A.2 容器规范摘要**")
	r.ln("")
	for _, c := range r.v.Containers {
		r.p("- `%s`\n  - file_sha256: `%s`\n  - memberset_sha256: `%s`\n  - pathset_sha256: `%s`\n",
			c.Path, c.FileSHA256, c.MemberSetSHA256, c.PathSetSHA256)
	}
	r.ln("")
}

// r_caseDir exposes the renderer case directory without broadening the struct
// literal surface.
func r_caseDir(r *reportRenderer) string { return r.caseDir }
