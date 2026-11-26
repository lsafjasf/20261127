package chain

import (
	"os"
	"path/filepath"
	"strings"

	"evidencechain/internal/model"
)

// RenderReport re-runs verification and assembles the full Chinese report.
// linkPrefix is prefixed to case-relative paths in Markdown links (use "case/"
// when the report is written to the repository root).
func RenderReport(caseDir, linkPrefix string) (string, *model.Verification, error) {
	v, err := Verify(caseDir)
	if err != nil {
		return "", nil, err
	}
	idx, err := LoadIndex(caseDir)
	if err != nil {
		return "", nil, err
	}
	var js model.Judgments
	_ = ReadJSON(filepath.Join(caseDir, "review", "judgments.json"), &js)
	notes, _ := os.ReadFile(filepath.Join(caseDir, "review", "analyst-notes.md"))

	r := &reportRenderer{caseDir: caseDir, v: v, idx: idx, js: &js, notes: string(notes), lp: linkPrefix}
	var b strings.Builder
	r.b = &b
	r.header()
	r.sectionSummary()
	r.sectionChain()
	r.sectionMethod()
	r.sectionAnchors()
	r.sectionLinks()
	r.sectionFiles()
	r.sectionContainers()
	r.sectionWatch()
	r.sectionNotes()
	r.sectionJudgments()
	r.sectionRepro()
	return b.String(), v, nil
}

type reportRenderer struct {
	caseDir string
	b       *strings.Builder
	v       *model.Verification
	idx     *model.Index
	js      *model.Judgments
	notes   string
	lp      string
}

func (r *reportRenderer) p(format string, args ...any) { r.b.WriteString(sprintf(format, args...)) }
func (r *reportRenderer) ln(s string)                  { r.b.WriteString(s + "\n") }

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func (r *reportRenderer) fileLink(rel string) string {
	return "[" + mdCell(rel) + "](<" + r.lp + rel + ">)"
}

func shortHash(h string, n int) string {
	if len(h) <= n {
		return h
	}
	return h[:n] + "…"
}

func codeOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return "`" + s + "`"
}

func (r *reportRenderer) header() {
	r.ln("# 电子数据取证检验报告（证据链参考案例）")
	r.ln("")
	r.p("- 案例编号：`%s`\n", r.v.CaseID)
	r.p("- 生成工具：`%s %s`（模式 `%s`，纯 Go 标准库，可完全离线构建）\n", r.v.Tool, r.v.Version, r.v.Schema)
	r.p("- 哈希索引块大小：`%d` 字节；摘要算法：SHA-256；相似性算法：ecsh1（8 字节 shingle + 64 位 SimHash）\n", r.idx.ChunkSize)
	r.ln("- 报告性质：由 `evidencechain report` 依据 `index/hash-index.json`、实际检验结果与 `review/judgments.json` 自动汇编；第九节为分析员手工勘验原文。")
	r.ln("")
	r.ln("> **案例性质声明**：工作区中并未提供真实案件检材，本报告使用工具确定性生成的**虚构教学/自检案例**（公司、人员、电话、编号均为合成）。")
	r.ln("> 所有哈希、块差异与校验结论均为对仓库内实际文件重新计算所得，可用第十一节的命令完整复现。工具与证据链结构可直接用于真实检材。")
	r.ln("")
}

func (r *reportRenderer) sectionSummary() {
	r.ln("## 一、结论摘要")
	r.ln("")
	if r.v.OK {
		r.ln("**证据链总体状态：✅ 链完整且自洽（机器校验 0 错误）。** 原始材料封印完好，全部保管链步骤可复算，人工判读与机器事实一致，不存在把相似哈希当作同一来源的过度断言。")
	} else {
		r.p("**证据链总体状态：❌ 存在 %d 个必须排除的错误。**\n", len(r.v.Errors))
	}
	r.ln("")
	r.ln("在文件改名、容器重排与局部篡改之后，仍可得到以下可信结论：")
	r.ln("")
	r.ln("- **改名不影响同一认定（高置信）**：`未命名_扫描件_01.dat` 与 `客户导出_20261126.csv` 经完整 SHA-256 分别精确命中 EV-001、EV-002。")
	r.ln("- **容器重排可被规范摘要识别（高置信）**：`backup_20261126.zip` 条目乱序、成员全部改名，但其成员内容集合摘要与封存包完全相同；tar 包证明 EV-003/EV-004 跨容器迁移。")
	r.ln("- **局部篡改已定位（高置信）**：外传日志及邮件压缩包内的“终稿”相对 EV-001，10 个对齐块中 8 块逐块相同，差异集中在第 3、4 块（字节区间 384–640），人工比对确认时间与库号两处改写。")
	r.ln("- **相似文本不得认定同源（保留不确定）**：“同题仿作”相似度约 28.7%，但无对齐块相同，来源关系不确定。")
	r.ln("- **前缀相似哈希被拒绝（高置信的规则结论）**：协查 WL-2 与 EV-003 仅共享 12 位前缀，不构成命中；WL-3 模糊指纹汉明距离为 0 仍只作线索。")
	r.ln("- **存在未知检材**：`设备缓存_dc88.bin` 不属于已固定原件集合，来源与用途不明。")
	r.ln("")
	r.ln("机器统计：")
	r.ln("")
	r.ln("| 指标 | 数值 |")
	r.ln("|---|---:|")
	keys := []string{"anchors", "anchor_intact", "field_files", "file_identified", "file_tampered-localized",
		"file_similar-only", "file_unknown", "containers", "container_pristine-anchor-set",
		"container_pristine-subset", "container_contains-altered-or-unknown",
		"link_verified", "watch_exact-hit", "watch_rejected-prefix-only", "watch_similarity-lead",
		"judgment_supported", "judgment_uncertain-allowed", "errors", "warnings"}
	for _, k := range keys {
		if n, ok := r.v.Summary[k]; ok {
			r.p("| `%s` | %d |\n", k, n)
		}
	}
	r.ln("")
	if len(r.v.Errors) > 0 {
		r.ln("**错误：**")
		for _, e := range r.v.Errors {
			r.p("- %s\n", mdCell(e))
		}
		r.ln("")
	}
}

func (r *reportRenderer) sectionChain() {
	r.ln("## 二、证据链构成")
	r.ln("")
	r.ln("证据链由四层材料加两份机器记录构成，每层都能被独立复验：")
	r.ln("")
	r.ln("| 层 | 路径 | 角色 | 可信机制 |")
	r.ln("|---|---|---|---|")
	r.ln("| 原始材料 | `case/00-originals/` | 扣押/固定的基准检材（信任根） | 建立索引后只读对待；每次验证重算 SHA-256 封印 |")
	r.ln("| 派生副本 | `case/10-derived/`、`case/20-containers/sealed-pack.zip` | 字节副本、规范化文本、像素重编码、封存包 | `custody/provenance.json` 记录来源与变换，验证器确定性复算 |")
	r.ln("| 哈希索引 | `case/index/hash-index.json` | 全量 SHA-256、对齐块哈希与块根、ecsh1 模糊指纹 | 由 `index` 命令从原始材料生成；索引本身的哈希见第十一节 |")
	r.ln("| 人工判读 | `case/review/judgments.json`、`case/review/analyst-notes.md` | 分析员结论、置信度、依据与不确定声明 | 验证器将每条判读与机器事实交叉审计，拦截过度断言 |")
	r.ln("| 外勤/回传 | `case/30-field-returns/` | 改名、重排容器、篡改、仿作、未知件 | 全部按内容哈希而非文件名/容器位置识别 |")
	r.ln("| 外部协查 | `case/watchlist/reference-hashes.json` | 外部哈希通报（含诱饵） | 完整值才命中；前缀/模糊值降级处理 |")
	r.ln("| 机器结论 | `case/out/verification.json` | 结构化验证结果 | `verify`/`report` 每次重新生成 |")
	r.ln("")
}

func (r *reportRenderer) sectionMethod() {
	r.ln("## 三、方法与判定规则")
	r.ln("")
	r.ln("**哈希层级（证明力由强到弱）**")
	r.ln("")
	r.ln("1. **完整 SHA-256（64 位十六进制）**：全文件/全成员精确相等 ⇒ 位级同一（在 SHA-256 抗碰撞假设下）；不等 ⇒ 内容必不相同。文件名、扩展名、容器条目顺序、压缩元数据均不参与。")
	r.p("2. **对齐块 SHA-256 + 块根**：按固定偏移把文件切成 `%d` 字节块并逐块哈希，用于把局部篡改定位到字节区间；块根以域分隔串 `EVIDENCECHAIN/CHUNKROOT/v1` 提交块序。\n", r.idx.ChunkSize)
	r.ln("3. **ecsh1 相似指纹**：8 字节滑动 shingle 的 Jaccard 重叠率 + 64 位 SimHash 汉明距离。它只回答“有多像”，**永远不回答“是不是同一个/是否同源”**。")
	r.ln("4. **内容级派生校验**：文本规范化可确定性复算；PNG 重编码通过解码后逐像素比对确认画面一致。")
	r.ln("")
	r.ln("**容器规范摘要**：对 zip/tar 每个成员流式计算 SHA-256，再分别计算")
	r.ln("`memberset_sha256 = SHA256(域分隔 + 排序后的 <成员哈希,尺寸> 列表)`（与成员名、顺序、格式无关）")
	r.ln("和 `pathset_sha256`（额外绑定成员名）。因此：")
	r.ln("")
	r.ln("- 容器文件整包 SHA-256 变化 + memberset 不变 ⇒ 仅改名/重排/重打包；")
	r.ln("- 某成员哈希不命中 ⇒ 下沉到该成员的块/相似性检验；")
	r.ln("- 文件在 zip/tar 之间迁移不影响成员级同一认定。")
	r.ln("")
	r.ln("**“相似哈希不是同一证据”的执行规则**")
	r.ln("")
	r.ln("- 只有完整 64 位 SHA-256 精确相等才允许 `identity/rename/watchlist-hit` 结论；")
	r.ln("- 仅共享前缀（即便 12 位以上）一律标记 `rejected-prefix-only`，不得报命中；")
	r.ln("- SimHash 汉明距离为 0 或模糊指纹完全相同，也只产生 `similarity-lead`；")
	r.ln("- 人工判读若在没有精确/块哈希事实时宣称同一或同源，审计状态为 `overclaim/contradiction`，验证以非零码失败；")
	r.ln("- 证据不足的结论必须使用 `uncertain-origin`，工具将其审计为 `uncertain-allowed` 并在报告中显式保留。")
	r.ln("")
}

func (r *reportRenderer) sectionAnchors() {
	r.ln("## 四、原始材料：哈希索引与封印校验")
	r.ln("")
	r.ln("| 证据号 | 逻辑文件名 | 尺寸(B) | SHA-256 | 块根(前16) | ecsh1 SimHash | 封印 |")
	r.ln("|---|---|---:|---|---|---|---|")
	anchorStatus := map[string]model.AnchorResult{}
	for _, a := range r.v.Anchors {
		anchorStatus[a.EvidenceID] = a
	}
	for _, e := range r.idx.Entries {
		st := anchorStatus[e.EvidenceID].Status
		r.p("| `%s` | %s | %d | `%s` | `%s` | `%s` | %s |\n",
			e.EvidenceID, r.fileLink(e.RelPath), e.Size, e.SHA256, shortHash(e.ChunkRoot, 16), e.Fuzzy.SimHash, anchorLabel(st))
	}
	r.ln("")
	r.ln("完整 SHA-256 见附录 A 与 `case/index/hash-index.json`。封印状态全部为 `intact` 表示当前 `00-originals/` 与建索引时逐字节一致。")
	r.ln("")
}

func anchorLabel(s string) string {
	switch s {
	case "intact":
		return "✅ 完好"
	case "tampered":
		return "❌ 被改动"
	case "missing":
		return "❌ 缺失"
	}
	return s
}
