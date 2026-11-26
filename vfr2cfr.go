// vfr2cfr 在 VFR 成片渲染前生成完整的 CFR 封装规划：
// 源帧映射、重复/丢弃策略、音频总时长、字幕位置。
// 所有时间用有理数精确表示，避免浮点误差污染帧边界判定。
package main

import (
	"fmt"
)

// Rational 是精确有理数（秒），Num/Den 已约分，Den > 0。
type Rational struct {
	Num int64
	Den int64
}

func gcd(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func NewRational(num, den int64) Rational {
	if den < 0 {
		num, den = -num, -den
	}
	g := gcd(num, den)
	return Rational{Num: num / g, Den: den / g}
}

// Mul 返回 a*b（约分）。
func (a Rational) Mul(b Rational) Rational {
	g1 := gcd(a.Num, b.Den)
	g2 := gcd(b.Num, a.Den)
	return NewRational(a.Num/g1*(b.Num/g2), a.Den/g2*(b.Den/g1))
}

// Div 返回 a/b。
func (a Rational) Div(b Rational) Rational {
	return a.Mul(NewRational(b.Den, b.Num))
}

// Sub 返回 a-b。
func (a Rational) Sub(b Rational) Rational {
	return NewRational(a.Num*b.Den-b.Num*a.Den, a.Den*b.Den)
}

func (a Rational) Float64() float64 {
	return float64(a.Num) / float64(a.Den)
}

func (a Rational) String() string {
	return fmt.Sprintf("%.6fs", a.Float64())
}

// roundNearInf 四舍五入（半值远离零），对应 ffmpeg AV_ROUND_NEAR_INF。
func roundNearInf(r Rational) int64 {
	num, den := r.Num, r.Den
	if num >= 0 {
		return (num + den/2) / den
	}
	return -((-num + den/2) / den)
}

// floorDiv 向下取整。
func floorRat(r Rational) int64 {
	q := r.Num / r.Den
	if r.Num%r.Den != 0 && r.Num < 0 {
		q--
	}
	return q
}

// ceilRat 向上取整。
func ceilRat(r Rational) int64 {
	return -floorRat(Rational{Num: -r.Num, Den: r.Den})
}

// SrcFrame 是一个 VFR 源帧，PTS 为显示时刻（秒）。
type SrcFrame struct {
	Index int
	PTS   Rational
}

// FrameAction 是源帧在 CFR 输出中的命运。
type FrameAction int

const (
	ActionNormal     FrameAction = iota // 恰好映射到 1 个输出帧
	ActionDuplicated                    // 被复制，填充多个输出帧
	ActionDropped                       // 被丢弃，不出现于输出
)

func (a FrameAction) String() string {
	switch a {
	case ActionDuplicated:
		return "DUP"
	case ActionDropped:
		return "DROP"
	default:
		return "MAP"
	}
}

// FrameDecision 记录一个源帧的映射结果。
type FrameDecision struct {
	Src         SrcFrame
	Action      FrameAction
	FirstOut    int64 // 首个输出帧序号（DROP 时为 -1）
	OutCount    int64 // 占用的输出帧数（DROP 时为 0）
	ExtraCopies int64 // 额外复制份数 = OutCount-1
}

// Plan 是一次完整的 VFR→CFR 封装规划。
type Plan struct {
	FPS         Rational // 目标恒定帧率
	Decisions   []FrameDecision
	OutFrames   int64    // 输出总帧数
	OutDuration Rational // 输出视频时长 = OutFrames / FPS
}

// PlanCFR 按 ffmpeg fps 滤镜语义生成映射：
// 源帧 j 填充输出槽 [round(pts_j*fps), round(pts_{j+1}*fps)），
// 末帧填到 round(srcDuration*fps)。槽数 0 → 丢弃，>1 → 复制。
// srcDuration 为源总时长（末帧 PTS + 末帧时长）。
func PlanCFR(frames []SrcFrame, srcDuration, fps Rational) *Plan {
	p := &Plan{FPS: fps, Decisions: make([]FrameDecision, 0, len(frames))}
	slotOf := func(t Rational) int64 { return roundNearInf(t.Mul(fps)) }

	for j, f := range frames {
		first := slotOf(f.PTS)
		var end int64
		if j+1 < len(frames) {
			end = slotOf(frames[j+1].PTS)
		} else {
			end = slotOf(srcDuration)
		}
		d := FrameDecision{Src: f, FirstOut: first, OutCount: end - first}
		switch {
		case d.OutCount <= 0:
			d.Action = ActionDropped
			d.FirstOut, d.OutCount = -1, 0
		case d.OutCount > 1:
			d.Action = ActionDuplicated
			d.ExtraCopies = d.OutCount - 1
		}
		p.Decisions = append(p.Decisions, d)
	}
	p.OutFrames = slotOf(srcDuration)
	p.OutDuration = NewRational(p.OutFrames, 1).Div(fps)
	return p
}

// AudioPlan 描述随视频一起封装音频所需的总时长信息。
type AudioPlan struct {
	SrcDuration    Rational // 源音频时长（不变，不重采样）
	OutDuration    Rational // 封装后音频总时长（对齐到视频时长）
	Pad            Rational // 需要补静音的时长（OutDuration-SrcDuration，负值表示截断）
	SampleRate     int64
	TotalSamples   int64 // OutDuration * SampleRate
	FrameSize      int64 // 编码帧样本数（如 AAC 1024）
	TotalEncFrames int64 // TotalSamples 向上对齐到 FrameSize 后的编码帧数
}

// PlanAudio 音频流不重采样、不改 PTS，只按输出视频时长补齐/截断，
// 使容器内音视频总时长一致（CFR 容器要求流时长对齐）。
func PlanAudio(srcAudioDuration Rational, sampleRate, frameSize int64, p *Plan) AudioPlan {
	out := p.OutDuration
	total := roundNearInf(out.Mul(NewRational(sampleRate, 1)))
	encFrames := (total + frameSize - 1) / frameSize
	return AudioPlan{
		SrcDuration:    srcAudioDuration,
		OutDuration:    out,
		Pad:            out.Sub(srcAudioDuration),
		SampleRate:     sampleRate,
		TotalSamples:   total,
		FrameSize:      frameSize,
		TotalEncFrames: encFrames,
	}
}

// SubCue 是一条字幕 cue。
type SubCue struct {
	Index      int
	Start, End Rational // 源时间轴上的起止（秒）
	Text       string
}

// SubPlacement 是 cue 在 CFR 时间轴上的落位。
type SubPlacement struct {
	Cue        SubCue
	StartFrame int64    // 起始输出帧 = floor(start*fps)
	EndFrame   int64    // 结束输出帧 = ceil(end*fps)，保证覆盖原区间
	Start, End Rational // 吸附到帧边界后的起止时间
}

// PlanSubtitles 把字幕起止吸附到 CFR 帧网格：
// 开始向下取整、结束向上取整，保证显示区间不缩水。
func PlanSubtitles(cues []SubCue, p *Plan) []SubPlacement {
	out := make([]SubPlacement, 0, len(cues))
	for _, c := range cues {
		sf := floorRat(c.Start.Mul(p.FPS))
		ef := ceilRat(c.End.Mul(p.FPS))
		if ef > p.OutFrames {
			ef = p.OutFrames // 不超出成片末尾
		}
		if sf < 0 {
			sf = 0
		}
		out = append(out, SubPlacement{
			Cue:        c,
			StartFrame: sf,
			EndFrame:   ef,
			Start:      NewRational(sf, 1).Div(p.FPS),
			End:        NewRational(ef, 1).Div(p.FPS),
		})
	}
	return out
}

func ms(v int64) Rational { return NewRational(v, 1000) }

func main() {
	// 一段混合 VFR 素材：前 5 帧约 24fps，中间 3 帧高帧率（会触发丢弃），
	// 后段帧间隔拉长（会触发复制）。目标 CFR：30000/1001 ≈ 29.97fps。
	fps := NewRational(30000, 1001)
	frames := []SrcFrame{
		{0, ms(0)}, {1, ms(42)}, {2, ms(83)}, {3, ms(125)}, {4, ms(167)}, // ~24fps
		{5, ms(208)}, {6, ms(217)}, {7, ms(225)}, // 突发高帧率 → 丢弃
		{8, ms(400)}, {9, ms(550)}, {10, ms(700)}, // 间隔拉长 → 复制
	}
	srcDuration := ms(767) // 末帧 PTS + 末帧时长

	plan := PlanCFR(frames, srcDuration, fps)

	fmt.Printf("目标 CFR: %d/%d fps (%.3f)\n", fps.Num, fps.Den, fps.Float64())
	fmt.Printf("输出总帧数: %d  输出视频时长: %s\n\n", plan.OutFrames, plan.OutDuration)

	fmt.Println("== 源帧映射 ==")
	for _, d := range plan.Decisions {
		if d.Action == ActionDropped {
			fmt.Printf("src#%-3d pts=%-10s → DROP（无输出槽）\n", d.Src.Index, d.Src.PTS)
		} else {
			fmt.Printf("src#%-3d pts=%-10s → out[%d..%d]  %s x%d\n",
				d.Src.Index, d.Src.PTS, d.FirstOut, d.FirstOut+d.OutCount-1,
				d.Action, d.OutCount)
		}
	}

	fmt.Println("\n== 每个被复制的源帧 ==")
	for _, d := range plan.Decisions {
		if d.Action == ActionDuplicated {
			fmt.Printf("DUP  src#%d pts=%s 额外复制 %d 份（共占 %d 个输出帧）\n",
				d.Src.Index, d.Src.PTS, d.ExtraCopies, d.OutCount)
		}
	}
	fmt.Println("\n== 每个被丢弃的源帧 ==")
	for _, d := range plan.Decisions {
		if d.Action == ActionDropped {
			fmt.Printf("DROP src#%d pts=%s（被相邻帧覆盖，不进入输出）\n",
				d.Src.Index, d.Src.PTS)
		}
	}

	audio := PlanAudio(ms(750), 48000, 1024, plan)
	fmt.Println("\n== 音频总时长 ==")
	fmt.Printf("源音频时长: %s  封装后总时长: %s  补静音: %s\n",
		audio.SrcDuration, audio.OutDuration, audio.Pad)
	fmt.Printf("采样率 %dHz → 总样本 %d，AAC 帧(1024) %d 个\n",
		audio.SampleRate, audio.TotalSamples, audio.TotalEncFrames)

	subs := []SubCue{
		{0, ms(10), ms(120), "第一句台词"},
		{1, ms(205), ms(230), "高帧率区的短字幕"},
		{2, ms(410), ms(760), "跨越复制段的台词"},
	}
	fmt.Println("\n== 字幕位置（吸附到 CFR 帧网格）==")
	for _, sp := range PlanSubtitles(subs, plan) {
		fmt.Printf("cue#%d 帧[%3d..%3d]  %s → %s  %q\n",
			sp.Cue.Index, sp.StartFrame, sp.EndFrame, sp.Start, sp.End, sp.Cue.Text)
	}
}
