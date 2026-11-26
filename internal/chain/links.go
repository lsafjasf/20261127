package chain

import (
	"os"
	"path/filepath"

	"evidencechain/internal/containerx"
	"evidencechain/internal/derive"
	"evidencechain/internal/digestx"
	"evidencechain/internal/model"
)

func (v *verifier) resolveRef(token string) (string, []byte, error) {
	rel := token
	if e, ok := v.anchors[token]; ok {
		rel = e.RelPath
	}
	b, err := os.ReadFile(filepath.Join(v.dir, filepath.FromSlash(rel)))
	return rel, b, err
}

func (v *verifier) checkLinks() {
	if v.prov == nil {
		return
	}
	for _, link := range v.prov.Links {
		lr := model.LinkResult{ID: link.ID, To: link.To, Relation: link.Relation}
		toBytes, err := os.ReadFile(filepath.Join(v.dir, filepath.FromSlash(link.To)))
		if err != nil {
			lr.Status = "broken"
			lr.Detail = "派生目标缺失或不可读: " + err.Error()
			v.err("保管链 %s 断裂：目标 %s 不可读", link.ID, link.To)
			v.res.Links = append(v.res.Links, lr)
			continue
		}

		switch link.Relation {
		case "byte-copy":
			_, src, serr := v.resolveRef(link.From[0])
			if serr != nil {
				lr.Status, lr.Detail = "broken", "来源不可读: "+serr.Error()
			} else if digestx.SHA256Bytes(src) == digestx.SHA256Bytes(toBytes) {
				lr.Status, lr.Detail = "verified", "字节副本 SHA-256 与来源一致"
			} else {
				lr.Status, lr.Detail = "broken", "字节副本哈希与来源不一致"
			}
		case "normalize-eol":
			_, src, serr := v.resolveRef(link.From[0])
			switch {
			case serr != nil:
				lr.Status, lr.Detail = "broken", "来源不可读: "+serr.Error()
			case string(derive.NormalizeEOL(src)) == string(toBytes):
				lr.Status, lr.Detail = "verified", "规范化结果可由来源确定性复算"
			default:
				lr.Status, lr.Detail = "broken", "规范化结果与 derive.NormalizeEOL 复算不一致"
			}
		case "png-reencode":
			_, src, serr := v.resolveRef(link.From[0])
			if serr != nil {
				lr.Status, lr.Detail = "broken", "来源不可读: "+serr.Error()
			} else if ok, detail, perr := derive.PNGPixelsEqual(src, toBytes); perr != nil {
				lr.Status, lr.Detail = "broken", "PNG 解码失败: "+perr.Error()
			} else if ok {
				lr.Status, lr.Detail = "verified", "字节哈希不同，但"+detail
			} else {
				lr.Status, lr.Detail = "broken", "像素不一致: "+detail
			}
		case "zip-pack", "tar-pack":
			lr = v.checkPackLink(link, lr)
		default:
			lr.Status, lr.Detail = "unsupported", "未知派生关系类型: "+link.Relation
			v.warn("保管链 %s 使用了校验器不认识的关系 %q", link.ID, link.Relation)
		}

		if lr.Status == "broken" {
			v.err("保管链 %s 校验失败：%s", link.ID, lr.Detail)
		}
		v.res.Links = append(v.res.Links, lr)
	}
}

func (v *verifier) checkPackLink(link model.Link, lr model.LinkResult) model.LinkResult {
	_, _, entries, err := containerx.ReadEntries(filepath.Join(v.dir, filepath.FromSlash(link.To)))
	if err != nil {
		lr.Status, lr.Detail = "broken", "容器不可解析: "+err.Error()
		return lr
	}
	gotSet := containerx.MemberSetDigest(containerx.MembersFromEntries(entries))
	expected := make([]containerx.Member, 0, len(link.From))
	for _, ref := range link.From {
		_, b, err := v.resolveRef(ref)
		if err != nil {
			lr.Status, lr.Detail = "broken", "来源不可读: "+err.Error()
			return lr
		}
		expected = append(expected, containerx.Member{Size: int64(len(b)), SHA256: digestx.SHA256Bytes(b)})
	}
	wantSet := containerx.MemberSetDigest(expected)
	if gotSet == wantSet {
		lr.Status, lr.Detail = "verified", "容器成员集合摘要与声明的来源集合一致（与顺序/名称无关）"
	} else {
		lr.Status, lr.Detail = "broken", "容器成员集合与声明来源不符"
	}
	return lr
}
