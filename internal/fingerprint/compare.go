package fingerprint

import (
	"fmt"
	"sort"
)

// Verdict 是两文件比对结论。
type Verdict string

const (
	// Identical 指纹全等:内容与容器均未变。
	Identical Verdict = "IDENTICAL"
	// Remux 正常重封装:载荷一致,布局变化(盒序/chunk/stco/填充),
	// 元数据可同可变。常见于转封装、faststart 移动 moov。
	Remux Verdict = "REMUX"
	// MetadataModified 元数据修改:载荷与布局均一致,仅元数据变化
	// (标题/标签/时间戳等),样本表与 mdat 布局未动。
	MetadataModified Verdict = "METADATA_MODIFIED"
	// ContentReplaced 真实帧替换:载荷指纹不一致,存在样本级差异。
	ContentReplaced Verdict = "CONTENT_REPLACED"
)

// TrackDiff 记录一轨的样本级差异证据。
type TrackDiff struct {
	TrackID        uint32 `json:"track_id"`
	Handler        string `json:"handler"`
	CodecA         string `json:"codec_a"`
	CodecB         string `json:"codec_b"`
	SamplesA       int    `json:"samples_a"`
	SamplesB       int    `json:"samples_b"`
	FirstDiffIndex int    `json:"first_diff_index"` // -1 表示仅数量不同
	DiffIndices    []int  `json:"diff_indices,omitempty"`
	Truncated      bool   `json:"truncated"` // DiffIndices 是否被截断
}

// Report 是比对报告,可直接序列化为证据。
type Report struct {
	Verdict        Verdict     `json:"verdict"`
	Explanation    string      `json:"explanation"`
	PayloadEqual   bool        `json:"payload_equal"`
	StructureEqual bool        `json:"structure_equal"`
	LayoutEqual    bool        `json:"layout_equal"`
	MetadataEqual  bool        `json:"metadata_equal"`
	WholeFileEqual bool        `json:"whole_file_equal"`
	TrackDiffs     []TrackDiff `json:"track_diffs,omitempty"`
}

const maxDiffIndices = 64

// Compare 比对两个已解析媒体,给出判定与证据。
func Compare(a, b *Media) *Report {
	r := &Report{
		PayloadEqual:   a.FP.Payload == b.FP.Payload,
		StructureEqual: a.FP.Structure == b.FP.Structure,
		LayoutEqual:    a.FP.Layout == b.FP.Layout,
		MetadataEqual:  a.FP.Metadata == b.FP.Metadata,
		WholeFileEqual: a.FP.WholeFile == b.FP.WholeFile,
	}
	switch {
	case !r.PayloadEqual:
		r.Verdict = ContentReplaced
		r.TrackDiffs = diffTracks(a.Tracks, b.Tracks)
		r.Explanation = "载荷指纹不一致:音视频样本内容发生变化,属真实帧替换/篡改"
	case r.StructureEqual:
		r.Verdict = Identical
		r.Explanation = "四级指纹全部一致:内容与容器均未变化"
	case r.LayoutEqual && !r.MetadataEqual:
		r.Verdict = MetadataModified
		r.Explanation = "载荷与布局指纹一致,仅元数据指纹变化:标签/时间戳等元数据被修改"
	default:
		r.Verdict = Remux
		r.Explanation = "载荷指纹一致、布局指纹变化:盒顺序/chunk 布局/偏移表改变,属正常重封装"
	}
	return r
}

// diffTracks 按 (Handler,Codec) 对齐两文件的轨,逐样本比对哈希。
func diffTracks(a, b []TrackDigest) []TrackDiff {
	key := func(t TrackDigest) string { return t.Handler + "/" + t.Codec }
	bm := make(map[string][]TrackDigest)
	for _, t := range b {
		bm[key(t)] = append(bm[key(t)], t)
	}
	var diffs []TrackDiff
	for _, ta := range a {
		lst := bm[key(ta)]
		if len(lst) == 0 {
			diffs = append(diffs, TrackDiff{
				TrackID: ta.TrackID, Handler: ta.Handler,
				CodecA: ta.Codec, SamplesA: len(ta.Samples),
				FirstDiffIndex: 0,
			})
			continue
		}
		tb := lst[0]
		bm[key(ta)] = lst[1:]
		if ta.Hash == tb.Hash {
			continue
		}
		diffs = append(diffs, diffOneTrack(ta, tb))
	}
	for _, lst := range bm {
		for _, tb := range lst {
			diffs = append(diffs, TrackDiff{
				TrackID: tb.TrackID, Handler: tb.Handler,
				CodecB: tb.Codec, SamplesB: len(tb.Samples),
				FirstDiffIndex: 0,
			})
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Handler < diffs[j].Handler })
	return diffs
}

func diffOneTrack(a, b TrackDigest) TrackDiff {
	d := TrackDiff{
		TrackID: a.TrackID, Handler: a.Handler,
		CodecA: a.Codec, CodecB: b.Codec,
		SamplesA: len(a.Samples), SamplesB: len(b.Samples),
		FirstDiffIndex: -1,
	}
	n := len(a.Samples)
	if len(b.Samples) < n {
		n = len(b.Samples)
	}
	for i := 0; i < n; i++ {
		sa, sb := a.Samples[i], b.Samples[i]
		if sa.SHA256 != sb.SHA256 || sa.DTS != sb.DTS || sa.Duration != sb.Duration {
			if d.FirstDiffIndex < 0 {
				d.FirstDiffIndex = i
			}
			if len(d.DiffIndices) < maxDiffIndices {
				d.DiffIndices = append(d.DiffIndices, i)
			} else {
				d.Truncated = true
			}
		}
	}
	if d.FirstDiffIndex < 0 && len(a.Samples) != len(b.Samples) {
		d.FirstDiffIndex = n // 前缀一致,长度不同(增删帧)
	}
	return d
}

// String 输出人类可读的判定摘要。
func (r *Report) String() string {
	s := fmt.Sprintf("判定: %s\n依据: %s\n", r.Verdict, r.Explanation)
	for _, d := range r.TrackDiffs {
		s += fmt.Sprintf("  轨%d(%s): 样本 %d→%d, 首个差异样本 #%d, 差异样本数≥%d\n",
			d.TrackID, d.Handler, d.SamplesA, d.SamplesB,
			d.FirstDiffIndex, len(d.DiffIndices))
	}
	return s
}
