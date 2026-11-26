package chain

import (
	"path/filepath"

	"evidencechain/internal/digestx"
	"evidencechain/internal/fuzzyx"
	"evidencechain/internal/model"
)

// status vocabulary shared by files and container members.
const (
	stIdentified  = "identified"
	stTampered    = "tampered-localized"
	stSimilarOnly = "similar-only"
	stUnknown     = "unknown"
)

// classifySimilarFloor is the minimum shingle overlap at which an object is
// reported as "similar-only". It is a lead threshold, not an identity rule.
const classifySimilarFloor = 18.0

type classification struct {
	status       string
	evidenceID   string
	renamedFrom  string
	candidateID  string
	fuzzy        *model.FuzzyVerdict
	diffs        []model.BlockDiff
	matchedBlock int
	totalBlock   int
	detail       string
}

func (v *verifier) classify(data []byte, objectName string) classification {
	sum := digestx.SHA256Bytes(data)
	if entry, ok := v.byHash[sum]; ok {
		c := classification{status: stIdentified, evidenceID: entry.EvidenceID}
		if objectName != "" && objectName != entry.LogicalName &&
			objectName != filepath.Base(entry.RelPath) {
			c.renamedFrom = entry.LogicalName
		}
		c.detail = "完整 SHA-256 与基准一致：位级同一（改名不影响内容认定）"
		return c
	}

	// No exact identity. Find the closest anchor by similarity signal.
	var bestID string
	var best fr
	for id, ab := range v.anchorBytes {
		r := fuzzyx.Compare(data, ab)
		if bestID == "" || r.JaccardPercent > best.j.JaccardPercent {
			bestID, best = id, fr{j: r}
		}
	}
	c := classification{}
	// A near-zero overlap means the "closest" anchor is statistically
	// meaningless; do not present it as a candidate.
	if bestID != "" && best.j.JaccardPercent >= 1.0 {
		c.candidateID = bestID
		c.fuzzy = &model.FuzzyVerdict{
			JaccardPercent: best.j.JaccardPercent,
			HammingBits:    best.j.HammingBits,
			Lead:           best.j.IsLead(),
		}
		diffs, matched, total := v.blockDiff(data, bestID)
		c.diffs, c.matchedBlock, c.totalBlock = diffs, matched, total
		changed := countChanged(diffs)
		ratio := 0.0
		if total > 0 {
			ratio = float64(matched) / float64(total)
		}
		// Aligned exact-block matches are strong localization evidence;
		// fuzzy overlap alone never reaches this branch.
		if changed > 0 && matched >= 2 && ratio >= 0.5 {
			c.status = stTampered
			c.detail = "整文件 SHA-256 不一致，但多数对齐块仍与基准逐块相同，可定位局部篡改"
			return c
		}
	}

	if c.fuzzy != nil && c.fuzzy.JaccardPercent >= classifySimilarFloor {
		c.status = stSimilarOnly
		c.detail = "仅存在相似性信号；没有完整哈希或足够对齐块支持同一/篡改结论"
		return c
	}

	c.status = stUnknown
	c.detail = "未命中任何基准哈希，且无显著相似性；来源不明"
	return c
}

type fr struct{ j fuzzyx.Result }

func countChanged(diffs []model.BlockDiff) int {
	n := 0
	for _, d := range diffs {
		if d.Status != "match" {
			n++
		}
	}
	return n
}

// blockDiff compares aligned chunks of data against one anchor. It returns
// only non-matching block records plus match/total counters.
func (v *verifier) blockDiff(data []byte, anchorID string) ([]model.BlockDiff, int, int) {
	entry := v.anchors[anchorID]
	want := make(map[int]model.ChunkRecord, len(entry.Chunks))
	for _, ch := range entry.Chunks {
		want[ch.Index] = ch
	}
	got := digestx.ChunkBytes(data, v.idx.ChunkSize)
	n := len(got)
	if len(entry.Chunks) > n {
		n = len(entry.Chunks)
	}
	var diffs []model.BlockDiff
	matched := 0
	for i := 0; i < n; i++ {
		w, wok := want[i]
		if i < len(got) {
			g := got[i]
			if wok && g.SHA256 == w.SHA256 {
				matched++
				continue
			}
			status := "differ"
			if !wok {
				status = "extra"
			}
			diffs = append(diffs, model.BlockDiff{
				ChunkIndex: i, Start: g.Start, End: g.End, Status: status,
			})
			continue
		}
		// anchor has block i but questioned data is shorter
		diffs = append(diffs, model.BlockDiff{
			ChunkIndex: i, Start: w.Start, End: w.End, Status: "missing",
		})
	}
	return diffs, matched, n
}
