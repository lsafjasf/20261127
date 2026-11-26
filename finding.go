package main

import "sort"

// Finding is one located anomaly with a classification and evidence.
type Finding struct {
	ID         string         `json:"id,omitempty"`
	Key        string         `json:"-"`
	Kind       string         `json:"kind"`
	Title      string         `json:"title_zh"`
	Severity   string         `json:"severity"` // confirmed | likely | suspected | signal | info
	Confidence float64        `json:"confidence"`
	Stage      Stage          `json:"stage"`
	Window     int            `json:"window"`
	TimeS      float64        `json:"time_s"`
	PTS        *float64       `json:"pts_s,omitempty"`
	Seq        int64          `json:"seq,omitempty"`
	HasSeq     bool           `json:"has_seq,omitempty"`
	FrameIndex int            `json:"frame_index,omitempty"`
	Frames     int            `json:"frames,omitempty"`
	Anomaly    bool           `json:"anomaly"`
	Evidence   map[string]any `json:"evidence,omitempty"`
	rank       float64        `json:"-"`
	hasRank    bool
}

// Classification kinds.
const (
	KindCaptureDrop      = "capture_drop"              // 采集丢帧
	KindEncodeDuplicate  = "encode_duplicate"          // 编码重复帧
	KindPlayerSkip       = "player_skip"               // 播放器跳帧
	KindEncodeDrop       = "encode_drop"               // 编码/封装丢帧（额外相邻类别）
	KindFrameLoss        = "frame_loss"                // 录制文件缺帧，根因待佐证
	KindPlayerShort      = "player_shortage"           // 播放端帧数不足，待录制端比对
	KindDownstreamShort  = "downstream_shortage"       // 上游缺帧传导，非播放器责任
	KindSuspectedCapture = "suspected_capture_restamp" // 时间戳重建后的疑似采集丢失
	KindStaticRepeat     = "static_repeat"             // 静止画面下的相同帧（非故障）
	KindSceneCut         = "scene_cut"                 // 镜头切换（非掉帧）
)

var kindTitle = map[string]string{
	KindCaptureDrop:      "采集丢帧",
	KindEncodeDuplicate:  "编码重复帧",
	KindPlayerSkip:       "播放器跳帧",
	KindEncodeDrop:       "编码/封装丢帧",
	KindFrameLoss:        "录制时间窗缺帧（疑似采集丢帧）",
	KindPlayerShort:      "播放端渲染帧不足",
	KindDownstreamShort:  "播放端帧数随上游缺失（非播放器跳帧）",
	KindSuspectedCapture: "疑似采集丢帧后重建时间戳",
	KindStaticRepeat:     "静止画面重复帧（通常非故障）",
	KindSceneCut:         "镜头切换（非掉帧）",
}

// kindRank controls which classification replaces another when evidence merges.
var kindRank = map[string]int{
	KindCaptureDrop:      5,
	KindPlayerSkip:       5,
	KindEncodeDuplicate:  5,
	KindEncodeDrop:       4,
	KindFrameLoss:        2,
	KindPlayerShort:      2,
	KindDownstreamShort:  1,
	KindSuspectedCapture: 2,
	KindStaticRepeat:     1,
	KindSceneCut:         1,
}

type bag struct {
	order []string
	m     map[string]*Finding
}

func newBag() *bag { return &bag{m: map[string]*Finding{}} }

func (b *bag) add(f Finding) *Finding {
	if f.Key == "" {
		f.Key = f.Kind + ":auto"
	}
	if old, ok := b.m[f.Key]; ok {
		mergeFinding(old, &f)
		return old
	}
	if f.Title == "" {
		f.Title = kindTitle[f.Kind]
	}
	f.rank = f.TimeS
	b.m[f.Key] = &f
	b.order = append(b.order, f.Key)
	return &f
}

func (b *bag) get(key string) *Finding { return b.m[key] }

// mergeInto moves all evidence/classification from srcKey into dstKey and drops src.
// It reports whether both keys existed and a merge happened.
func (b *bag) mergeInto(dstKey, srcKey string) bool {
	dst, ok1 := b.m[dstKey]
	src, ok2 := b.m[srcKey]
	if !ok1 || !ok2 {
		return false
	}
	mergeFinding(dst, src)
	delete(b.m, srcKey)
	for i, k := range b.order {
		if k == srcKey {
			b.order = append(b.order[:i], b.order[i+1:]...)
			break
		}
	}
	return true
}

func mergeFinding(dst, src *Finding) {
	if kindRank[src.Kind] > kindRank[dst.Kind] {
		dst.Kind = src.Kind
		dst.Title = kindTitle[src.Kind]
	}
	if severityRank(src.Severity) > severityRank(dst.Severity) {
		dst.Severity = src.Severity
	}
	if src.Confidence > dst.Confidence {
		dst.Confidence = src.Confidence
	}
	if src.Stage != StageUnknown {
		dst.Stage = src.Stage
	}
	if src.PTS != nil {
		dst.PTS = src.PTS
	}
	if src.HasSeq && !dst.HasSeq {
		dst.Seq, dst.HasSeq = src.Seq, true
	}
	if src.Frames > dst.Frames {
		dst.Frames = src.Frames
	}
	if src.Anomaly {
		dst.Anomaly = true
	}
	if dst.Evidence == nil && src.Evidence != nil {
		dst.Evidence = map[string]any{}
	}
	for k, v := range src.Evidence {
		if _, exists := dst.Evidence[k]; !exists {
			dst.Evidence[k] = v
		}
	}
	if src.hasRank && (!dst.hasRank || src.rank < dst.rank) {
		dst.rank, dst.hasRank = src.rank, true
	}
}

func severityRank(s string) int {
	switch s {
	case "confirmed":
		return 4
	case "likely":
		return 3
	case "suspected":
		return 2
	case "signal":
		return 1
	default:
		return 0
	}
}

func (b *bag) sorted() []*Finding {
	out := make([]*Finding, 0, len(b.order))
	for _, k := range b.order {
		if f, ok := b.m[k]; ok {
			f.ID = ""
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := out[i].rank, out[j].rank
		if !out[i].hasRank {
			ri = out[i].TimeS
		}
		if !out[j].hasRank {
			rj = out[j].TimeS
		}
		if ri != rj {
			return ri < rj
		}
		if out[i].Window != out[j].Window {
			return out[i].Window < out[j].Window
		}
		if stageOrder(out[i].Stage) != stageOrder(out[j].Stage) {
			return stageOrder(out[i].Stage) < stageOrder(out[j].Stage)
		}
		return false
	})
	for i, f := range out {
		f.ID = "F" + itoa(i+1)
	}
	return out
}

func stageOrder(s Stage) int {
	switch s {
	case StageCapture:
		return 0
	case StageRecord:
		return 1
	case StagePlayer:
		return 2
	default:
		return 3
	}
}

func firstAnomaly(fs []*Finding) *Finding {
	for _, f := range fs {
		if f.Anomaly && f.Severity != "info" {
			return f
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
