package chain

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"evidencechain/internal/containerx"
	"evidencechain/internal/digestx"
	"evidencechain/internal/model"
)

type verifier struct {
	dir         string
	idx         *model.Index
	anchors     map[string]model.IndexEntry
	byHash      map[string]model.IndexEntry
	anchorBytes map[string][]byte
	prov        *model.Provenance
	judgments   *model.Judgments
	wl          *model.Watchlist
	res         *model.Verification
	targets     map[string]bool
}

// Verify runs the complete chain verification for a case directory.
func Verify(caseDir string) (*model.Verification, error) {
	idx, err := LoadIndex(caseDir)
	if err != nil {
		return nil, err
	}
	v := &verifier{
		dir:         caseDir,
		idx:         idx,
		anchors:     map[string]model.IndexEntry{},
		byHash:      map[string]model.IndexEntry{},
		anchorBytes: map[string][]byte{},
		res: &model.Verification{
			Schema: model.Schema, Tool: model.ToolName, Version: model.ToolVersion,
			CaseID: idx.CaseID, OK: true, Summary: map[string]int{},
		},
		targets: map[string]bool{},
	}
	for _, e := range idx.Entries {
		v.anchors[e.EvidenceID] = e
		v.byHash[e.SHA256] = e
		b, err := os.ReadFile(filepath.Join(caseDir, filepath.FromSlash(e.RelPath)))
		if err == nil {
			v.anchorBytes[e.EvidenceID] = b
		}
	}

	v.checkAnchors()
	v.loadOptionalRecords()
	v.checkLinks()
	v.scanFieldFiles()
	v.scanContainers()
	v.checkWatchlist()
	v.auditJudgments()
	v.checkCoverage()
	v.buildFindings()
	v.finalizeSummary()

	if len(v.res.Errors) > 0 {
		v.res.OK = false
	}
	return v.res, nil
}

func (v *verifier) rel(path string) string {
	r, err := filepath.Rel(v.dir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(r)
}

func (v *verifier) err(format string, args ...any) {
	v.res.Errors = append(v.res.Errors, sprintf(format, args...))
}

func (v *verifier) warn(format string, args ...any) {
	v.res.Warnings = append(v.res.Warnings, sprintf(format, args...))
}

func (v *verifier) checkAnchors() {
	for _, e := range v.idx.Entries {
		ar := model.AnchorResult{EvidenceID: e.EvidenceID, RelPath: e.RelPath}
		p := filepath.Join(v.dir, filepath.FromSlash(e.RelPath))
		sum, size, err := digestx.SHA256File(p)
		switch {
		case os.IsNotExist(err):
			ar.Status, ar.Detail = "missing", "原始材料缺失"
			v.err("锚点 %s 文件缺失: %s", e.EvidenceID, e.RelPath)
		case err != nil:
			ar.Status, ar.Detail = "missing", err.Error()
			v.err("锚点 %s 读取失败: %v", e.EvidenceID, err)
		case !digestx.EqualDigest(sum, e.SHA256) || size != e.Size:
			ar.Status, ar.Detail = "tampered", "当前 SHA-256/尺寸与哈希索引不一致，信任基准被改动"
			v.err("锚点 %s 封印校验失败（索引 %s… / 现状 %s…）", e.EvidenceID, short(e.SHA256), short(sum))
		default:
			ar.Status, ar.Detail = "intact", "SHA-256、尺寸与索引一致"
		}
		v.res.Anchors = append(v.res.Anchors, ar)
	}
}

func (v *verifier) loadOptionalRecords() {
	var p model.Provenance
	if err := ReadJSON(filepath.Join(v.dir, "custody", "provenance.json"), &p); err == nil {
		v.prov = &p
	}
	var j model.Judgments
	if err := ReadJSON(filepath.Join(v.dir, "review", "judgments.json"), &j); err == nil {
		v.judgments = &j
	} else {
		v.warn("未找到 review/judgments.json：缺少人工判读层，结论不完整")
	}
	var w model.Watchlist
	if err := ReadJSON(filepath.Join(v.dir, "watchlist", "reference-hashes.json"), &w); err == nil {
		v.wl = &w
	}
}

func (v *verifier) scanFieldFiles() {
	root := filepath.Join(v.dir, "30-field-returns")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if containerx.IsContainerPath(path) {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			v.err("读取失败 %s: %v", v.rel(path), rerr)
			return nil
		}
		rel := v.rel(path)
		c := v.classify(data, filepath.Base(path))
		fr := model.FileResult{
			Path: rel, Size: int64(len(data)), SHA256: digestx.SHA256Bytes(data),
			Status: c.status, MatchedEvidence: c.evidenceID, RenamedFrom: c.renamedFrom,
			Candidate: c.candidateID, Fuzzy: c.fuzzy, BlockDiffs: c.diffs, Detail: c.detail,
			MatchedBlocks: c.matchedBlock, TotalBlocks: c.totalBlock,
		}
		v.res.Files = append(v.res.Files, fr)
		v.targets["file:"+rel] = true
		return nil
	})
	sort.Slice(v.res.Files, func(i, j int) bool { return v.res.Files[i].Path < v.res.Files[j].Path })
}

func (v *verifier) scanContainers() {
	var paths []string
	for _, dir := range []string{"20-containers", "30-field-returns"} {
		root := filepath.Join(v.dir, dir)
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if d.Type().IsRegular() && containerx.IsContainerPath(path) {
				paths = append(paths, path)
			}
			return nil
		})
	}
	sort.Strings(paths)
	for _, p := range paths {
		v.inspectContainer(p)
	}
}

func (v *verifier) inspectContainer(path string) {
	rel := v.rel(path)
	format, fileSHA, entries, err := containerx.ReadEntries(path)
	if err != nil {
		v.err("容器解析失败 %s: %v", rel, err)
		v.res.Containers = append(v.res.Containers, model.ContainerResult{
			Path: rel, Status: "unreadable", Detail: err.Error(),
		})
		return
	}
	hashMembers := containerx.MembersFromEntries(entries)
	cr := model.ContainerResult{
		Path: rel, Format: format, FileSHA256: fileSHA,
		MemberSetSHA256: containerx.MemberSetDigest(hashMembers),
		PathSetSHA256:   containerx.PathSetDigest(hashMembers),
	}
	allAnchorMembers := make([]containerx.Member, 0, len(v.idx.Entries))
	for _, e := range v.idx.Entries {
		allAnchorMembers = append(allAnchorMembers, containerx.Member{Name: e.LogicalName, Size: e.Size, SHA256: e.SHA256})
	}
	allSet := containerx.MemberSetDigest(allAnchorMembers)

	allExact := true
	ids := map[string]bool{}
	for _, en := range entries {
		c := v.classify(en.Data, en.Name)
		mr := model.MemberResult{
			Name: en.Name, Size: int64(len(en.Data)), SHA256: digestx.SHA256Bytes(en.Data),
			Status: c.status, MatchedEvidence: c.evidenceID, RenamedFrom: c.renamedFrom,
			Candidate: c.candidateID, Fuzzy: c.fuzzy, BlockDiffs: c.diffs,
			MatchedBlocks: c.matchedBlock, TotalBlocks: c.totalBlock,
		}
		if c.evidenceID != "" {
			ids[c.evidenceID] = true
		} else {
			allExact = false
		}
		cr.Members = append(cr.Members, mr)
		v.targets["member:"+rel+"#"+en.Name] = true
	}
	switch {
	case cr.MemberSetSHA256 == allSet:
		cr.Status = "pristine-anchor-set"
		cr.MatchesAnchorSet = strings.Join(anchorIDs(v.idx), ",")
		cr.Detail = "成员内容集合摘要与全部基准原件完全一致：仅存在改名/重排/重打包"
	case allExact && len(entries) > 0:
		cr.Status = "pristine-subset"
		cr.Detail = "所有成员均精确命中基准哈希，为原件集合的子集（可能跨容器迁移）"
	default:
		cr.Status = "contains-altered-or-unknown"
		cr.Detail = "存在未精确命中基准的成员，需要篡改定位或相似性分析"
	}
	v.res.Containers = append(v.res.Containers, cr)
	v.targets["container:"+rel] = true
}

func anchorIDs(idx *model.Index) []string {
	out := make([]string, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		out = append(out, e.EvidenceID)
	}
	return out
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
