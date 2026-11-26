package main

import (
	"math"
	"strings"
)

// Stage identifies one observation point of the live pipeline.
type Stage string

const (
	StageCapture Stage = "capture" // 采集端
	StageRecord  Stage = "record"  // 录制/编码文件
	StagePlayer  Stage = "player"  // 播放/渲染端
	StageUnknown Stage = "unknown"
)

// ParseStage understands english and chinese stage aliases.
func ParseStage(s string) Stage {
	v := strings.ToLower(strings.TrimSpace(s))
	switch {
	case v == "":
		return StageUnknown
	case containsAny(v, "captur", "cam", "source", "采集", "摄像头"):
		return StageCapture
	case containsAny(v, "player", "playback", "render", "present", "播放", "渲染"):
		return StagePlayer
	case containsAny(v, "record", "encod", "mux", "file", "录制", "录像", "编码"):
		return StageRecord
	}
	return StageUnknown
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// StageLabel returns the chinese display label for a stage.
func StageLabel(s Stage) string {
	switch s {
	case StageCapture:
		return "采集端"
	case StageRecord:
		return "录制端"
	case StagePlayer:
		return "播放端"
	default:
		return "未知阶段"
	}
}

// Frame is one observed frame (or an explicit drop marker).
type Frame struct {
	Stage  Stage `json:"stage,omitempty"`
	Line   int   `json:"-"`
	Seq    int64 `json:"seq,omitempty"`
	HasSeq bool  `json:"-"`

	PTS    float64 `json:"pts,omitempty"`
	HasPTS bool    `json:"-"`

	Wall    float64 `json:"wall_s,omitempty"`
	HasWall bool    `json:"-"`

	Sig Sig `json:"-"`

	// Advance is a precomputed normalized visual distance to the predecessor.
	Advance    float64 `json:"advance,omitempty"`
	HasAdvance bool    `json:"-"`

	Dropped    bool `json:"dropped,omitempty"`
	HasDropped bool `json:"-"`
}

// Trace is one stage's ordered observations.
type Trace struct {
	Stage   Stage    `json:"stage"`
	Source  string   `json:"source,omitempty"`
	Frames  []*Frame `json:"frames"`
	Markers []*Frame `json:"markers,omitempty"`

	// Axis is the chosen time axis: "pts" or "wall".
	Axis       string  `json:"axis,omitempty"`
	AxisOrigin float64 `json:"axis_origin_s,omitempty"`
	FPS        float64 `json:"fps_estimated,omitempty"`
}

// Sources bundles the optional vantage points. At least one trace is required.
type Sources struct {
	Capture *Trace
	Record  *Trace
	Player  *Trace
}

func (s Sources) any() bool {
	return s.Capture != nil || s.Record != nil || s.Player != nil
}

func (s Sources) byStage(st Stage) *Trace {
	switch st {
	case StageCapture:
		return s.Capture
	case StageRecord:
		return s.Record
	case StagePlayer:
		return s.Player
	}
	return nil
}

// axisTime returns the value of the selected time axis for a frame.
func (t *Trace) axisTime(f *Frame) (float64, bool) {
	switch t.Axis {
	case "wall":
		if f.HasWall {
			return f.Wall, true
		}
		if f.HasPTS {
			return f.PTS, true
		}
	case "pts":
		if f.HasPTS {
			return f.PTS, true
		}
		if f.HasWall {
			return f.Wall, true
		}
	default:
		if f.HasPTS {
			return f.PTS, true
		}
		if f.HasWall {
			return f.Wall, true
		}
	}
	return 0, false
}

// relTime returns seconds relative to the axis origin.
func (t *Trace) relTime(f *Frame) float64 {
	v, ok := t.axisTime(f)
	if !ok {
		return math.NaN()
	}
	return v - t.AxisOrigin
}
