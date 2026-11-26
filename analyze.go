package main

import (
	"fmt"
	"math"
)

// Report is the full analysis result, JSON-serializable.
type Report struct {
	Schema     string               `json:"schema"`
	WindowSec  float64              `json:"window_s"`
	Thresholds map[string]float64   `json:"thresholds"`
	Traces     []TraceSummary       `json:"traces"`
	Windows    map[string][]*Window `json:"windows"`
	Findings   []*Finding           `json:"findings"`
	First      *Finding             `json:"first_anomaly"`
	Summary    map[string]int       `json:"summary"`
	Notes      []string             `json:"notes,omitempty"`
}

// TraceSummary describes one loaded trace.
type TraceSummary struct {
	Stage   Stage   `json:"stage"`
	Source  string  `json:"source,omitempty"`
	Frames  int     `json:"frames"`
	Markers int     `json:"drop_markers,omitempty"`
	Axis    string  `json:"time_axis"`
	FPS     float64 `json:"fps_median"`
	SpanS   float64 `json:"span_s"`
	WithSig int     `json:"with_signature"`
	WithAdv int     `json:"with_advance_metric"`
}

// Analyze runs window accounting plus cross-stage classification.
func Analyze(src Sources, opt Options) (*Report, error) {
	if opt.WindowSec <= 0 {
		opt.WindowSec = DefaultOptions().WindowSec
	}
	if opt.DupBits < 0 {
		opt.DupBits = 2
	}
	if opt.JumpRatio <= 0 {
		opt.JumpRatio = 3
	}
	if opt.MotionFloor <= 0 {
		opt.MotionFloor = DefaultOptions().MotionFloor
	}
	if opt.SceneCut <= 0 {
		opt.SceneCut = 0.6
	}
	if opt.Clock == "" {
		opt.Clock = "auto"
	}
	if !src.any() {
		return nil, fmt.Errorf("没有输入轨迹")
	}

	order := []Stage{StageCapture, StageRecord, StagePlayer}
	anas := map[Stage]*traceAna{}
	rep := &Report{
		Schema:    "framedoctor/v1",
		WindowSec: opt.WindowSec,
		Thresholds: map[string]float64{
			"dup_bits":       float64(opt.DupBits),
			"jump_ratio":     opt.JumpRatio,
			"motion_floor":   round4(opt.MotionFloor),
			"scene_cut_norm": opt.SceneCut,
		},
		Windows: map[string][]*Window{},
		Summary: map[string]int{},
	}

	for _, st := range order {
		tr := src.byStage(st)
		if tr == nil {
			continue
		}
		tr.Stage = st
		a, err := prepareTrace(tr, opt)
		if err != nil {
			return nil, err
		}
		anas[st] = a
		rep.Windows[string(st)] = a.wins
		sum := TraceSummary{
			Stage: st, Source: tr.Source, Frames: len(tr.Frames),
			Markers: len(tr.Markers), Axis: tr.Axis, FPS: round2(a.fps),
			SpanS: round3(a.rel[len(a.rel)-1]),
		}
		for _, f := range tr.Frames {
			if f.Sig.Valid() {
				sum.WithSig++
			}
			if f.HasAdvance {
				sum.WithAdv++
			}
		}
		rep.Traces = append(rep.Traces, sum)
		if sum.WithSig == 0 && sum.WithAdv == 0 {
			rep.Notes = append(rep.Notes, fmt.Sprintf(
				"%s 轨迹缺少 hash/phash/diff 等画面推进列：只能核对帧数，无法判定编码重复与画面跳变",
				StageLabel(st)))
		}
	}

	b := newBag()
	for _, st := range order {
		if a := anas[st]; a != nil {
			detectDeficits(st, a, b)
			detectDuplicateRuns(st, a, opt, b)
			detectJumps(st, a, opt, b)
		}
	}
	reconcile(src, anas, opt, b)

	rep.Findings = b.sorted()
	rep.First = firstAnomaly(rep.Findings)
	for _, f := range rep.Findings {
		rep.Summary[f.Kind]++
	}
	if rep.First == nil {
		rep.Summary["clean"] = 1
	}
	return rep, nil
}

func deficitKey(st Stage, win int) string {
	return fmt.Sprintf("loss:%s:w%d", st, win)
}

// detectDeficits turns per-window expected/actual shortfalls into findings.
func detectDeficits(st Stage, a *traceAna, b *bag) {
	for _, w := range a.wins {
		loss := int(math.Round(w.Want - float64(w.Got)))
		if loss < 1 && w.MarkedDropped == 0 {
			continue
		}
		n := loss
		if w.MarkedDropped > n {
			n = w.MarkedDropped
		}
		f := Finding{
			Key: deficitKey(st, w.Index), Kind: KindFrameLoss, Stage: st,
			Window: w.Index, TimeS: w.Start, Frames: n, Anomaly: true,
			Evidence: map[string]any{
				"want": w.Want, "got": w.Got, "missing_est": loss,
				"marked_dropped": w.MarkedDropped,
				"fresh":          w.Fresh, "advance_frame_units": w.AdvanceUnits,
			},
		}
		if len(w.FrameIdxs) > 0 {
			fi := w.FrameIdxs[0]
			f.FrameIndex = fi
			fr := a.trace.Frames[fi]
			if fr.HasSeq {
				f.Seq, f.HasSeq = fr.Seq, true
			}
		}
		switch st {
		case StageCapture:
			f.Kind = KindCaptureDrop
			f.Severity = "likely"
			f.Confidence = 0.85
			if w.MarkedDropped > 0 {
				f.Severity = "confirmed"
				f.Confidence = 0.97
			}
		case StagePlayer:
			f.Kind = KindPlayerShort
			f.Severity = "likely"
			f.Confidence = 0.7
		default:
			f.Severity = "likely"
			f.Confidence = 0.7
		}
		if w.MarkedDropped > 0 {
			seqs := []int64{}
			for _, m := range a.trace.Markers {
				if mr, ok := a.trace.axisTime(m); ok {
					wi := int(math.Floor((mr-a.trace.AxisOrigin)/a.winDur() + 1e-9))
					if wi == w.Index && m.HasSeq {
						seqs = append(seqs, m.Seq)
					}
				}
			}
			if len(seqs) > 0 {
				f.Evidence["dropped_seq"] = seqs
				f.Seq, f.HasSeq = seqs[0], true
			}
		}
		b.add(f)
	}
}

func (a *traceAna) winDur() float64 {
	if len(a.wins) >= 2 {
		return a.wins[1].Start - a.wins[0].Start
	}
	return 1
}

// detectDuplicateRuns finds consecutive zero-advancement edges (same picture).
func detectDuplicateRuns(st Stage, a *traceAna, opt Options, b *bag) {
	if st == StagePlayer {
		// Player traces inherit encoded duplicates; record side owns that call.
		return
	}
	n := len(a.trace.Frames)
	i := 1
	for i < n {
		if !a.hasEdge[i] || a.edges[i] > a.dupEps+1e-12 {
			i++
			continue
		}
		s := i // first frame that is a duplicate copy
		r := 0
		vals := []float64{}
		for i < n && a.hasEdge[i] && a.edges[i] <= a.dupEps+1e-12 {
			r++
			vals = append(vals, round4(a.edges[i]))
			i++
		}
		amidMotion := a.base >= opt.MotionFloor
		f := Finding{
			Key:    fmt.Sprintf("dup:%s:idx:%d", st, s),
			Window: a.frameWin[s], TimeS: round3(a.rel[s]),
			FrameIndex: s, Frames: r,
			Evidence: map[string]any{
				"extra_copies": r, "edge_norm": vals,
				"motion_baseline": round4(a.base),
			},
		}
		if fr := a.trace.Frames[s]; fr.HasSeq {
			f.Seq, f.HasSeq = fr.Seq, true
		}
		if !amidMotion {
			f.Kind = KindStaticRepeat
			f.Severity = "info"
			f.Confidence = 0.9
			f.Anomaly = false
		} else if st == StageRecord || st == StageUnknown {
			f.Kind = KindEncodeDuplicate
			f.Severity = "likely"
			f.Confidence = 0.85
			f.Anomaly = true
			f.Stage = StageRecord
		} else {
			f.Kind = KindEncodeDuplicate
			f.Severity = "suspected"
			f.Confidence = 0.55
			f.Anomaly = true
			f.Stage = st
			f.Evidence["observed_at_stage"] = string(st)
		}
		b.add(f)
	}
}

// detectJumps finds anomalously large single-frame visual leaps.
func detectJumps(st Stage, a *traceAna, opt Options, b *bag) {
	if a.base <= 1e-9 {
		return
	}
	n := len(a.trace.Frames)
	for i := 1; i < n; i++ {
		if !a.hasEdge[i] {
			continue
		}
		e := a.edges[i]
		if e < opt.JumpRatio*a.base-1e-9 || e <= a.dupEps {
			continue
		}
		wi := a.frameWin[i]
		evidence := map[string]any{
			"edge_norm": round4(e), "ratio_to_baseline": round2(e / a.base),
		}
		// Attach as corroborating evidence to a co-located deficit finding.
		attached := false
		for _, candW := range []int{wi, wi - 1} {
			if candW < 0 {
				continue
			}
			if f := b.get(deficitKey(st, candW)); f != nil {
				if f.Evidence == nil {
					f.Evidence = map[string]any{}
				}
				f.Evidence["jump_at_s"] = round3(a.rel[i])
				f.Evidence["jump_norm"] = round4(e)
				f.Evidence["jump_ratio"] = round2(e / a.base)
				attached = true
			}
		}
		// A jump right after a zero-edge run is the encoder catching up;
		// it belongs to the duplicate finding, not a restamped capture loss.
		dupAttached := false
		run := 0
		for k := i - 1; k >= 1 && a.hasEdge[k] && a.edges[k] <= a.dupEps+1e-12; k-- {
			run++
		}
		if run > 0 {
			s := i - run
			if df := b.get(fmt.Sprintf("dup:%s:idx:%d", st, s)); df != nil {
				if df.Evidence == nil {
					df.Evidence = map[string]any{}
				}
				df.Evidence["catchup_jump_at_s"] = round3(a.rel[i])
				df.Evidence["catchup_jump_ratio"] = round2(e / a.base)
				dupAttached = true
			}
		}
		if e >= opt.SceneCut {
			b.add(Finding{
				Key: fmt.Sprintf("cut:%s:idx:%d", st, i), Kind: KindSceneCut,
				Severity: "info", Confidence: 0.8, Anomaly: false, Stage: st,
				Window: wi, TimeS: round3(a.rel[i]), FrameIndex: i,
				Evidence: evidence,
			})
			continue
		}
		if !attached && !dupAttached && st == StageRecord {
			// Smooth regenerated timestamps can hide capture loss behind a jump.
			b.add(Finding{
				Key:  fmt.Sprintf("restamp:%s:idx:%d", st, i),
				Kind: KindSuspectedCapture, Severity: "suspected",
				Confidence: 0.55, Anomaly: true, Stage: StageRecord,
				Window: wi, TimeS: round3(a.rel[i]), FrameIndex: i,
				Evidence: evidence,
			})
		}
	}
}

func floatPtr(v float64) *float64 { return &v }
