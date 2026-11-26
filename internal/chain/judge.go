package chain

import (
	"strings"

	"evidencechain/internal/model"
)

type targetFacts struct {
	kind     string
	status   string
	hasFuzzy bool
	blockLoc bool
	exists   bool
}

func (v *verifier) resolveTarget(target string) targetFacts {
	switch {
	case strings.HasPrefix(target, "file:"):
		p := strings.TrimPrefix(target, "file:")
		for _, f := range v.res.Files {
			if f.Path == p {
				return targetFacts{kind: "file", status: f.Status, hasFuzzy: f.Fuzzy != nil, blockLoc: len(nonMatchDiffs(f.BlockDiffs)) > 0, exists: true}
			}
		}
	case strings.HasPrefix(target, "container:"):
		p := strings.TrimPrefix(target, "container:")
		for _, c := range v.res.Containers {
			if c.Path == p {
				return targetFacts{kind: "container", status: c.Status, exists: true}
			}
		}
	case strings.HasPrefix(target, "member:"):
		rest := strings.TrimPrefix(target, "member:")
		hashIdx := strings.Index(rest, "#")
		if hashIdx < 0 {
			break
		}
		cp, mn := rest[:hashIdx], rest[hashIdx+1:]
		for _, c := range v.res.Containers {
			if c.Path != cp {
				continue
			}
			for _, m := range c.Members {
				if m.Name == mn {
					return targetFacts{kind: "member", status: m.Status, hasFuzzy: m.Fuzzy != nil, blockLoc: len(nonMatchDiffs(m.BlockDiffs)) > 0, exists: true}
				}
			}
		}
	case strings.HasPrefix(target, "watch:"):
		id := strings.TrimPrefix(target, "watch:")
		for _, w := range v.res.Watchlist {
			if w.Label == id || strings.HasPrefix(w.Label, id+" ") {
				return targetFacts{kind: "watch", status: w.Status, exists: true}
			}
		}
	case strings.HasPrefix(target, "link:"):
		id := strings.TrimPrefix(target, "link:")
		for _, l := range v.res.Links {
			if l.ID == id {
				return targetFacts{kind: "link", status: l.Status, exists: true}
			}
		}
	case strings.HasPrefix(target, "anchor:"):
		id := strings.TrimPrefix(target, "anchor:")
		for _, a := range v.res.Anchors {
			if a.EvidenceID == id {
				return targetFacts{kind: "anchor", status: a.Status, exists: true}
			}
		}
	}
	return targetFacts{}
}

func nonMatchDiffs(ds []model.BlockDiff) []model.BlockDiff {
	var out []model.BlockDiff
	for _, d := range ds {
		if d.Status != "match" {
			out = append(out, d)
		}
	}
	return out
}

func (v *verifier) auditJudgments() {
	if v.judgments == nil {
		return
	}
	for _, j := range v.judgments.Items {
		jr := model.JudgeResult{ID: j.ID, Target: j.Target, Verdict: j.Verdict}
		f := v.resolveTarget(j.Target)
		if !f.exists {
			jr.Status = "unknown-target"
			jr.Detail = "判读指向的对象在检验结果中不存在"
			v.err("人工判读 %s 指向未知目标 %s", j.ID, j.Target)
			v.res.Judgments = append(v.res.Judgments, jr)
			continue
		}
		v.auditOne(j, f, &jr)
		if jr.Status == "overclaim" || jr.Status == "contradiction" {
			v.err("人工判读 %s 未通过审计：%s（%s）", j.ID, jr.Status, jr.Detail)
		}
		v.res.Judgments = append(v.res.Judgments, jr)
	}
}

func (v *verifier) auditOne(j model.Judgment, f targetFacts, jr *model.JudgeResult) {
	set := func(status, detail string) { jr.Status, jr.Detail = status, detail }
	switch j.Verdict {
	case "identity-confirmed", "rename-confirmed":
		if (f.kind == "file" || f.kind == "member") && f.status == stIdentified {
			if !hasBasis(j.Basis, "exact-sha256") {
				set("contradiction", "结论为同一认定，但依据中缺少 exact-sha256 记录")
				return
			}
			set("supported", "完整哈希精确命中，同一认定有机器事实支撑")
			return
		}
		set("overclaim", "对象没有完整 SHA-256 精确命中；相似哈希或人工印象不能支持同一/同源结论")
	case "tamper-localized":
		if (f.kind == "file" || f.kind == "member") && f.status == stTampered && f.blockLoc {
			if !hasBasis(j.Basis, "chunk-sha256") {
				set("contradiction", "篡改结论缺少 chunk-sha256 块哈希依据")
				return
			}
			set("supported", "块级哈希差异已定位，局部篡改结论成立")
			return
		}
		set("overclaim", "缺少块级差异事实，不能给出已定位篡改的结论")
	case "container-rearranged":
		if f.kind == "container" && (f.status == "pristine-anchor-set" || f.status == "pristine-subset") {
			set("supported", "成员内容集合摘要证明容器仅改名/重排，内容未变")
			return
		}
		set("contradiction", "容器状态不是 pristine 集合，不能按单纯重排结案")
	case "container-set-confirmed":
		if f.kind == "container" && (f.status == "pristine-anchor-set" || f.status == "pristine-subset") {
			set("supported", "容器成员集合与基准原件集合（或其子集）一致，封存/迁移关系成立")
			return
		}
		set("contradiction", "容器成员集合未能与基准集合对应")
	case "container-contains-tamper":
		if f.kind == "container" && f.status == "contains-altered-or-unknown" {
			set("supported", "容器内存在未精确命中的成员，已结合成员级块差异/相似性检验处理")
			return
		}
		set("contradiction", "机器检验未显示该容器含异常成员")
	case "similarity-only":
		if (f.kind == "watch" && f.status == "similarity-lead") || f.hasFuzzy || f.status == stSimilarOnly {
			set("supported", "仅主张相似性；记录明确排除同一/同源推断，用法正确")
			return
		}
		set("contradiction", "该对象没有相似性信号，similarity-only 判读与事实不符")
	case "uncertain-origin":
		set("uncertain-allowed", "分析员明确保留不确定性；证据链允许并显式记录该结论")
	case "unknown-item":
		if f.status == stUnknown {
			set("supported", "对象无哈希命中且无显著相似性，未知结论与事实一致")
			return
		}
		set("contradiction", "对象并非 unknown 状态，不能按未知检材处理")
	case "watchlist-hit":
		if f.kind == "watch" && f.status == "exact-hit" {
			set("supported", "协查哈希完整精确命中")
			return
		}
		set("contradiction", "该协查项并非 exact-hit，不能报告哈希命中")
	case "watchlist-rejected":
		if f.kind == "watch" && (strings.HasPrefix(f.status, "rejected") || f.status == "no-match" || f.status == "malformed") {
			set("supported", "工具正确拒绝了把前缀/残缺/模糊哈希当作同一命中")
			return
		}
		set("contradiction", "该协查项状态不支持 watchlist-rejected 判读")
	case "watchlist-similarity-lead":
		if f.kind == "watch" && f.status == "similarity-lead" {
			set("supported", "协查模糊指纹仅产生相似线索；明确不得据此认定同一来源")
			return
		}
		set("contradiction", "该协查项没有 similarity-lead 事实")
	case "link-verified":
		if f.kind == "link" && f.status == "verified" {
			set("supported", "派生/保管步骤可复算验证")
			return
		}
		set("contradiction", "保管链步骤未处于 verified 状态")
	default:
		set("contradiction", "未知判读类型: "+j.Verdict)
	}
}

// checkCoverage warns about material objects without any human judgment.
func (v *verifier) checkCoverage() {
	if v.judgments == nil {
		return
	}
	judged := map[string]bool{}
	for _, j := range v.judgments.Items {
		judged[j.Target] = true
	}
	for _, f := range v.res.Files {
		if !judged["file:"+f.Path] {
			v.warn("外勤文件缺少人工判读: %s", f.Path)
		}
	}
	for _, c := range v.res.Containers {
		if !judged["container:"+c.Path] {
			v.warn("容器缺少人工判读: %s", c.Path)
		}
	}
	for _, w := range v.res.Watchlist {
		id := strings.SplitN(w.Label, " ", 2)[0]
		if !judged["watch:"+id] {
			v.warn("协查项缺少人工判读: %s", id)
		}
	}
}
