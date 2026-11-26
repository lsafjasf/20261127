package casegen

import (
	"evidencechain/internal/digestx"
	"evidencechain/internal/fuzzyx"
	"evidencechain/internal/model"
)

func (f *fixture) writeWatchlist() error {
	ev2Hash := digestx.SHA256Bytes(f.ev2)
	ev3Hash := digestx.SHA256Bytes(f.ev3)
	// Construct a bulletin digest that shares a 12-hex prefix with EV-003
	// but is otherwise unrelated — the "similar hash" trap.
	decoyTail := digestx.SHA256Bytes([]byte("external-bulletin/typo/decoy"))
	lookalike := ev3Hash[:12] + decoyTail[:52]

	wl := model.Watchlist{
		Schema: model.Schema,
		CaseID: CaseID,
		Source: "虚构协查通报 XB-2026-1127（教学数据）",
		Entries: []model.WatchlistHash{
			{
				Label: "WL-1 客户名单完整哈希", Kind: "sha256",
				Value: ev2Hash, Evidence: "EV-002",
				Note: "通报要求排查的完整 SHA-256",
			},
			{
				Label: "WL-2 设备固件疑似哈希（转录可能有误）", Kind: "sha256-lookalike",
				Value: lookalike, Evidence: "EV-003",
				Note: "与 EV-003 共享 12 位前缀，但全值不同；不得据此认定同一",
			},
			{
				Label: "WL-3 值班日志模糊指纹", Kind: "fuzzy",
				Value: fuzzyx.Sum(f.ev1).SimHash, Evidence: "EV-001",
				Note: "仅用于发现相似材料，不能证明同一来源",
			},
			{
				Label: "WL-4 无关样本哈希", Kind: "sha256",
				Value: digestx.SHA256Bytes([]byte("this sample is not in the case")),
				Note:  "用于演示无命中",
			},
		},
	}
	return writeJSON(rel(f.dir, "watchlist", "reference-hashes.json"), wl)
}

func (f *fixture) writeProvenance() error {
	p := model.Provenance{
		Schema: model.Schema,
		CaseID: CaseID,
		Note:   "参考案例的保管与派生关系；时间为虚构的固定时间线。",
		Links: []model.Link{
			{
				ID: "L-01", From: []string{"EV-001"}, To: "10-derived/EV-001.字节副本.txt",
				Relation: "byte-copy", Operator: "analyst.he", At: "2026-11-26T09:10:00+08:00",
				Details: "位对位复制，哈希应完全一致",
			},
			{
				ID: "L-02", From: []string{"EV-001"}, To: "10-derived/EV-001.规范化.txt",
				Relation: "normalize-eol", Operator: "analyst.he", At: "2026-11-26T09:12:00+08:00",
				Details: "CRLF→LF、去除行尾空白；可用 derive.NormalizeEOL 复算",
			},
			{
				ID: "L-03", From: []string{"EV-002"}, To: "10-derived/EV-002.字节副本.csv",
				Relation: "byte-copy", Operator: "analyst.he", At: "2026-11-26T09:15:00+08:00",
			},
			{
				ID: "L-04", From: []string{"EV-004"}, To: "10-derived/EV-004.像素重编码.png",
				Relation: "png-reencode", Operator: "analyst.he", At: "2026-11-26T09:20:00+08:00",
				Details: "改变 PNG 压缩级别，字节哈希变化但解码像素应完全一致",
			},
			{
				ID: "L-05", From: []string{"EV-001", "EV-002", "EV-003", "EV-004"},
				To: "20-containers/sealed-pack.zip", Relation: "zip-pack",
				Operator: "analyst.he", At: "2026-11-26T09:30:00+08:00",
				Details: "封存打包；成员集合摘要应与四件原件一致，与条目顺序/压缩元数据无关",
			},
		},
	}
	return writeJSON(rel(f.dir, "custody", "provenance.json"), p)
}
