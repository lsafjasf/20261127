package main

import (
	"fmt"
	"math"
	"sort"
)

// Options controls windowing and classification thresholds.
type Options struct {
	FPS         float64
	WindowSec   float64
	DupBits     int
	JumpRatio   float64
	MotionFloor float64
	SceneCut    float64
	Clock       string // auto | pts | wall
}

// DefaultOptions returns conservative defaults for ~25-60 fps material.
func DefaultOptions() Options {
	return Options{
		FPS:         0,
		WindowSec:   1.0,
		DupBits:     2,
		JumpRatio:   3.0,
		MotionFloor: 2.0 / 64.0,
		SceneCut:    0.6,
		Clock:       "auto",
	}
}

// Window is the triple comparison unit: expected vs actual vs advancement.
type Window struct {
	Index         int      `json:"index"`
	Start         float64  `json:"start_s"`
	End           float64  `json:"end_s"`
	Want          float64  `json:"want_frames"`
	Got           int      `json:"actual_frames"`
	Missing       float64  `json:"missing_frames"`
	MarkedDropped int      `json:"marked_dropped,omitempty"`
	Dup           int      `json:"duplicate_frames"`
	Fresh         int      `json:"fresh_frames"`
	Advance       float64  `json:"advance_norm"`
	Baseline      float64  `json:"baseline_step_norm,omitempty"`
	AdvanceUnits  float64  `json:"advance_frame_units"`
	MaxEdge       float64  `json:"max_edge_norm,omitempty"`
	MaxEdgeAt     float64  `json:"max_edge_at_s,omitempty"`
	Jumps         int      `json:"jump_edges,omitempty"`
	Status        string   `json:"status"`
	Reasons       []string `json:"reasons,omitempty"`
	FrameIdxs     []int    `json:"frame_indexes,omitempty"`
}

type traceAna struct {
	trace    *Trace
	times    []float64 // raw axis value per sorted frame
	rel      []float64 // seconds relative to origin
	edges    []float64 // normalized distance from previous frame (-1 if n/a)
	hasEdge  []bool
	base     float64
	fps      float64
	dt       float64
	wins     []*Window
	frameWin []int
	dupEps   float64
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]float64(nil), xs...)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

func prepareTrace(t *Trace, opt Options) (*traceAna, error) {
	if t == nil || len(t.Frames) == 0 {
		return nil, fmt.Errorf("轨迹 %s 没有可用帧", t.Source)
	}
	chooseAxis(t, opt.Clock)

	// Stable sort by axis time; rows without time sort last but are rare.
	sort.SliceStable(t.Frames, func(i, j int) bool {
		ai, oki := t.axisTime(t.Frames[i])
		aj, okj := t.axisTime(t.Frames[j])
		if !oki {
			return false
		}
		if !okj {
			return true
		}
		if ai != aj {
			return ai < aj
		}
		return t.Frames[i].Line < t.Frames[j].Line
	})

	a := &traceAna{trace: t, dupEps: float64(opt.DupBits) / 64.0}
	n := len(t.Frames)
	a.times = make([]float64, n)
	a.rel = make([]float64, n)
	a.edges = make([]float64, n)
	a.hasEdge = make([]bool, n)

	var origin float64
	for i, f := range t.Frames {
		v, ok := t.axisTime(f)
		if !ok {
			return nil, fmt.Errorf("轨迹 %s 第 %d 行缺少可用时间戳", t.Source, f.Line)
		}
		if i == 0 {
			origin = v
		}
		a.times[i] = v
		a.rel[i] = v - origin
	}
	t.AxisOrigin = origin

	// Edge visual distances.
	var motionEdges, allEdges []float64
	for i := 1; i < n; i++ {
		e := edgeDistance(t.Frames[i-1], t.Frames[i])
		if e >= 0 {
			a.edges[i] = e
			a.hasEdge[i] = true
			allEdges = append(allEdges, e)
			if e > a.dupEps+1e-12 {
				motionEdges = append(motionEdges, e)
			}
		}
	}
	if b := median(motionEdges); b > 1e-9 {
		a.base = b
	} else {
		a.base = median(allEdges)
	}

	// FPS: explicit option wins, otherwise robust median cadence.
	if opt.FPS > 0 {
		a.fps = opt.FPS
	} else {
		var dts []float64
		for i := 1; i < n; i++ {
			if d := a.rel[i] - a.rel[i-1]; d > 1e-6 {
				dts = append(dts, d)
			}
		}
		if med := median(dts); med > 1e-6 {
			a.fps = 1.0 / med
		} else {
			a.fps = 30
		}
	}
	a.dt = 1.0 / a.fps
	t.FPS = a.fps

	a.buildWindows(opt)
	return a, nil
}

func chooseAxis(t *Trace, clock string) {
	hasPTS, hasWall := 0, 0
	for _, f := range t.Frames {
		if f.HasPTS {
			hasPTS++
		}
		if f.HasWall {
			hasWall++
		}
	}
	switch clock {
	case "pts":
		if hasPTS > 0 {
			t.Axis = "pts"
		} else {
			t.Axis = "wall"
		}
	case "wall":
		if hasWall > 0 {
			t.Axis = "wall"
		} else {
			t.Axis = "pts"
		}
	default:
		if hasPTS >= hasWall && hasPTS > 0 {
			t.Axis = "pts"
		} else {
			t.Axis = "wall"
		}
	}
}

func edgeDistance(prev, cur *Frame) float64 {
	if d := SigDistance(prev.Sig, cur.Sig); d >= 0 {
		return d
	}
	if cur.HasAdvance {
		return cur.Advance
	}
	if prev.HasAdvance {
		return prev.Advance
	}
	return -1
}

func (a *traceAna) buildWindows(opt Options) {
	w := opt.WindowSec
	n := len(a.trace.Frames)
	span := a.rel[n-1] + a.dt
	nWin := int(math.Ceil(span/w - 1e-9))
	if nWin < 1 {
		nWin = 1
	}
	a.wins = make([]*Window, nWin)
	a.frameWin = make([]int, n)
	for k := 0; k < nWin; k++ {
		start := float64(k) * w
		overlap := math.Min(w, math.Max(0, span-start))
		win := &Window{
			Index:    k,
			Start:    round3(start),
			End:      round3(start + w),
			Want:     round1(a.fps * overlap),
			Baseline: round4(a.base),
		}
		a.wins[k] = win
	}
	for i := 0; i < n; i++ {
		wi := int(math.Floor(a.rel[i]/w + 1e-9))
		if wi < 0 {
			wi = 0
		}
		if wi >= nWin {
			wi = nWin - 1
		}
		a.frameWin[i] = wi
		win := a.wins[wi]
		win.Got++
		win.FrameIdxs = append(win.FrameIdxs, i)

		if a.hasEdge[i] {
			e := a.edges[i]
			win.Advance += e
			if e > win.MaxEdge {
				win.MaxEdge = e
				win.MaxEdgeAt = round3(a.rel[i])
			}
			if e <= a.dupEps+1e-12 {
				win.Dup++
			}
			if a.base > 1e-9 && e >= opt.JumpRatio*a.base-1e-9 && e > a.dupEps {
				win.Jumps++
			}
		}
	}
	// Explicit drop markers count toward their time window.
	for _, m := range a.trace.Markers {
		v, ok := a.trace.axisTime(m)
		if !ok {
			continue
		}
		rel := v - a.trace.AxisOrigin
		wi := int(math.Floor(rel/w + 1e-9))
		if wi < 0 {
			wi = 0
		}
		if wi >= nWin {
			wi = nWin - 1
		}
		a.wins[wi].MarkedDropped++
	}
	for _, win := range a.wins {
		win.Missing = round1(math.Max(0, win.Want-float64(win.Got)))
		win.Fresh = win.Got - win.Dup
		if a.base > 1e-9 {
			win.AdvanceUnits = round2(win.Advance / a.base)
		}
		win.Advance = round4(win.Advance)
		win.MaxEdge = round4(win.MaxEdge)
		win.Reasons = win.reasons(opt)
		if len(win.Reasons) == 0 {
			win.Status = "ok"
		} else {
			win.Status = joinReasons(win.Reasons)
		}
	}
}

func (win *Window) reasons(opt Options) []string {
	var r []string
	if win.Want-float64(win.Got) >= 0.5 || win.MarkedDropped > 0 {
		r = append(r, "frame_loss")
	}
	if win.Dup > 0 {
		r = append(r, "duplicate")
	}
	if win.Jumps > 0 {
		r = append(r, "jump")
	}
	return r
}

func joinReasons(rs []string) string {
	out := ""
	for i, r := range rs {
		if i > 0 {
			out += "+"
		}
		out += r
	}
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
