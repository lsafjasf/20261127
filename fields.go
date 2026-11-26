package main

import (
	"strconv"
	"strings"
	"time"
)

// normalizeKey canonicalizes CSV/JSON field names for alias matching.
func normalizeKey(k string) string {
	k = strings.TrimSpace(strings.ToLower(k))
	r := strings.NewReplacer("-", "_", " ", "_", ".", "_", "/", "_")
	k = r.Replace(k)
	for strings.Contains(k, "__") {
		k = strings.ReplaceAll(k, "__", "_")
	}
	return k
}

func pickField(m map[string]string, aliases ...string) (string, bool) {
	for _, a := range aliases {
		if v, ok := m[a]; ok && strings.TrimSpace(v) != "" {
			return v, true
		}
	}
	return "", false
}

var seqAliases = []string{
	"seq", "frame", "frame_no", "frameno", "frame_number", "framenum",
	"frame_num", "frame_index", "frame_idx", "index", "n", "id",
	"picture", "pic", "decoded_frame", "f",
	"帧号", "帧序号", "序号", "帧编号", "帧数",
}

var ptsSecAliases = []string{
	"pts", "pts_s", "pts_sec", "pts_time", "pts_seconds", "dts", "dts_s",
	"dts_time", "time", "time_s", "t", "t_s", "timestamp", "ts",
	"时间戳", "时间", "帧时间", "pts时间", "录制时间",
}

var ptsMSAliases = []string{"pts_ms", "dts_ms", "t_ms", "time_ms", "timestamp_ms", "ts_ms"}

var wallSecAliases = []string{
	"wall", "wall_s", "wall_time", "wall_sec", "wallclock", "wall_clock",
	"capture_time", "capture_ts", "render_time", "render_ts",
	"present_time", "present_ts", "presentation_time", "real_time",
	"墙钟", "墙钟时间", "采集时间", "渲染时间", "呈现时间", "播放时间",
}

var wallMSAliases = []string{
	"wall_ms", "wall_time_ms", "capture_time_ms", "capture_ts_ms",
	"render_time_ms", "present_time_ms",
}

var hashAliases = []string{
	"hash", "phash", "dhash", "ahash", "image_hash", "frame_hash",
	"perceptual_hash", "sig", "signature", "md5", "framemd5", "digest",
	"哈希", "帧哈希", "感知哈希", "画面哈希",
}

var advanceAliases = []string{
	"diff", "advance", "advancement", "visual_advance", "motion",
	"motion_score", "delta_norm", "change", "change_score", "adv",
	"画面推进", "推进量", "画面变化", "变化量", "运动量", "差异度",
}

var hammingAliases = []string{"hamming", "hamming_bits", "dist_bits", "bits"}
var distanceAliases = []string{"distance", "dist"}

var droppedAliases = []string{
	"dropped", "drop", "is_dropped", "dropped_frame", "drop_frame",
	"skipped", "skip", "is_skipped", "player_dropped", "lost", "missing",
	"是否丢帧", "丢帧", "掉帧", "是否跳过", "跳帧标记",
}

func parseInt64(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return int64(f), nil
	}
	return strconv.ParseInt(v, 10, 64)
}

func parseBoolish(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "1", "true", "yes", "y", "t", "drop", "dropped", "skip", "skipped":
		return true
	}
	return false
}

// parseClockValue parses seconds, milliseconds, HH:MM:SS[.frac] or RFC3339.
// The msHint forces millisecond interpretation for bare numbers.
func parseClockValue(raw string, msHint bool) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, "T") || (len(s) >= 19 && strings.Contains(s, "-")) {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, s); err == nil {
				return float64(t.UnixNano()) / 1e9, true
			}
		}
	}
	if strings.Contains(s, ":") {
		if sec, ok := parseHMS(s); ok {
			return sec, true
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if msHint || f > 1e11 {
			f /= 1000.0
		}
		return f, true
	}
	return 0, false
}

func parseHMS(s string) (float64, bool) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var sec float64
	for i, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0, false
		}
		switch len(parts) - i {
		case 3:
			sec += v * 3600
		case 2:
			sec += v * 60
		case 1:
			sec += v
		}
	}
	return sec, true
}

// buildFrame converts a normalized field map into a Frame.
func buildFrame(m map[string]string, line int, defaultStage Stage) (*Frame, []string) {
	f := &Frame{Line: line, Stage: defaultStage}
	var warns []string

	if v, ok := pickField(m, "stage", "source", "origin", "from"); ok {
		if st := ParseStage(v); st != StageUnknown {
			f.Stage = st
		}
	}
	if v, ok := pickField(m, seqAliases...); ok {
		if n, err := parseInt64(v); err == nil {
			f.Seq, f.HasSeq = n, true
		}
	}
	if v, ok := pickField(m, ptsMSAliases...); ok {
		if sec, good := parseClockValue(v, true); good {
			f.PTS, f.HasPTS = sec, true
		}
	}
	if !f.HasPTS {
		if v, ok := pickField(m, ptsSecAliases...); ok {
			if sec, good := parseClockValue(v, false); good {
				f.PTS, f.HasPTS = sec, true
			}
		}
	}
	if v, ok := pickField(m, wallMSAliases...); ok {
		if sec, good := parseClockValue(v, true); good {
			f.Wall, f.HasWall = sec, true
		}
	}
	if !f.HasWall {
		if v, ok := pickField(m, wallSecAliases...); ok {
			if sec, good := parseClockValue(v, false); good {
				f.Wall, f.HasWall = sec, true
			}
		}
	}
	if v, ok := pickField(m, hashAliases...); ok {
		if sig, err := ParseSig(v); err == nil && sig.Valid() {
			f.Sig = sig
		}
	}
	if v, ok := pickField(m, advanceAliases...); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			f.Advance, f.HasAdvance = n, true
		}
	}
	if v, ok := pickField(m, hammingAliases...); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			f.Advance, f.HasAdvance = n/64.0, true
		}
	}
	if v, ok := pickField(m, distanceAliases...); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			if n > 1.0 { // raw bit count convention
				n /= 64.0
			}
			f.Advance, f.HasAdvance = n, true
		}
	}
	if v, ok := pickField(m, droppedAliases...); ok {
		f.Dropped, f.HasDropped = parseBoolish(v), true
	}
	return f, warns
}
