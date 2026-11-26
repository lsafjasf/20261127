package chain

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"evidencechain/internal/fuzzyx"
	"evidencechain/internal/model"
)

func (v *verifier) checkWatchlist() {
	if v.wl == nil {
		return
	}
	exact := map[string][]string{}
	for _, e := range v.idx.Entries {
		exact[e.SHA256] = append(exact[e.SHA256], "anchor:"+e.EvidenceID+"("+e.LogicalName+")")
	}
	for _, f := range v.res.Files {
		exact[f.SHA256] = append(exact[f.SHA256], "file:"+f.Path)
	}
	for _, c := range v.res.Containers {
		for _, m := range c.Members {
			exact[m.SHA256] = append(exact[m.SHA256], "member:"+c.Path+"#"+m.Name)
		}
	}

	for _, w := range v.wl.Entries {
		wr := model.WatchResult{Label: w.Label, Kind: w.Kind, Value: w.Value}
		switch w.Kind {
		case "fuzzy":
			v.checkFuzzyWatch(w, &wr)
		default:
			v.checkSHAWatch(w, &wr, exact)
		}
		v.res.Watchlist = append(v.res.Watchlist, wr)
	}
}

func (v *verifier) checkSHAWatch(w model.WatchlistHash, wr *model.WatchResult, exact map[string][]string) {
	val := strings.ToLower(strings.TrimSpace(w.Value))
	if len(val) != 64 {
		wr.Status = "malformed"
		wr.Detail = "不是 64 位完整 SHA-256；截断/残缺哈希不能用于同一认定"
		return
	}
	if _, err := hex.DecodeString(val); err != nil {
		wr.Status = "malformed"
		wr.Detail = "含非十六进制字符，哈希值无效"
		return
	}
	if where, ok := exact[val]; ok {
		wr.Status = "exact-hit"
		wr.Against = strings.Join(where, " ; ")
		wr.Detail = sprintf("完整 64 位哈希精确命中 %d 处（含基准原件与派生/回传位置）", len(where))
		return
	}
	// No exact hit: measure prefix similarity against anchors.
	bestID, common := "", 0
	for _, e := range v.idx.Entries {
		n := commonPrefixLen(val, e.SHA256)
		if n > common {
			bestID, common = e.EvidenceID, n
		}
	}
	if w.Kind == "sha256-lookalike" || common >= 8 {
		wr.Status = "rejected-prefix-only"
		wr.Against = bestID
		wr.Detail = sprintf("与 %s 仅有 %d 位十六进制前缀相同，全值不同；相似/前缀哈希不是同一证据", bestID, common)
		return
	}
	wr.Status = "no-match"
	wr.Detail = "全值无命中，亦无值得报告的前缀相似"
}

func (v *verifier) checkFuzzyWatch(w model.WatchlistHash, wr *model.WatchResult) {
	bestName, bestDist := "", 65
	consider := func(name string, b []byte) {
		r, err := fuzzyx.CompareHash(b, w.Value)
		if err != nil {
			return
		}
		if r.HammingBits < bestDist {
			bestName, bestDist = name, r.HammingBits
		}
	}
	for id, b := range v.anchorBytes {
		consider(id, b)
	}
	for _, f := range v.res.Files {
		if b, err := os.ReadFile(filepath.Join(v.dir, filepath.FromSlash(f.Path))); err == nil {
			consider(f.Path, b)
		}
	}
	if bestName == "" {
		wr.Status = "malformed"
		wr.Detail = "模糊指纹无法解析"
		return
	}
	wr.Against = bestName
	if bestDist <= 10 {
		wr.Status = "similarity-lead"
		wr.Detail = sprintf("SimHash 汉明距离 %d/64，仅说明相似；即使距离为 0 也不等于文件同一或同源", bestDist)
	} else {
		wr.Status = "no-match"
		wr.Detail = sprintf("最近 SimHash 汉明距离 %d/64，相似性不足", bestDist)
	}
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}
