package hdrframe

import (
	"fmt"
	"strconv"
	"strings"
)

func parseFPS(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 24, nil
	}
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		num, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid fps numerator %q: %w", s, err)
		}
		den, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil || den == 0 {
			return 0, fmt.Errorf("invalid fps denominator %q", s)
		}
		return num / den, nil
	}
	fps, err := strconv.ParseFloat(s, 64)
	if err != nil || fps <= 0 {
		return 0, fmt.Errorf("invalid fps %q", s)
	}
	return fps, nil
}

func parseTimecode(tc string, fps float64) (float64, error) {
	tc = strings.TrimSpace(tc)
	if tc == "" {
		return 0, nil
	}
	sep := ":"
	if strings.Contains(tc, ";") {
		sep = ";"
	}
	parts := strings.FieldsFunc(tc, func(r rune) bool {
		return r == ':' || r == ';'
	})
	if len(parts) == 0 || len(parts) > 4 {
		return 0, fmt.Errorf("invalid timecode %q", tc)
	}
	var h, m, s, f float64
	var err error
	switch len(parts) {
	case 4:
		h, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timecode hour in %q", tc)
		}
		m, _ = strconv.ParseFloat(parts[1], 64)
		s, _ = strconv.ParseFloat(parts[2], 64)
		f, _ = strconv.ParseFloat(parts[3], 64)
	case 3:
		m, _ = strconv.ParseFloat(parts[0], 64)
		s, _ = strconv.ParseFloat(parts[1], 64)
		f, _ = strconv.ParseFloat(parts[2], 64)
	case 2:
		s, _ = strconv.ParseFloat(parts[0], 64)
		f, _ = strconv.ParseFloat(parts[1], 64)
	default:
		return strconv.ParseFloat(parts[0], 64)
	}
	_ = sep
	return h*3600 + m*60 + s + f/fps, nil
}

func formatTimecode(seconds, fps float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalFrames := int64(seconds*fps + 1e-9)
	frames := totalFrames % int64(fps+0.5)
	totalSecs := totalFrames / int64(fps+0.5)
	secs := totalSecs % 60
	mins := (totalSecs / 60) % 60
	hours := totalSecs / 3600
	return fmt.Sprintf("%02d:%02d:%02d:%02d", hours, mins, secs, frames)
}

func targetTime(t Target, fps float64) (float64, string, error) {
	if t.PTSSeconds != 0 {
		return t.PTSSeconds, t.Timecode, nil
	}
	if t.TimeSeconds != 0 {
		return t.TimeSeconds, t.Timecode, nil
	}
	if t.PTS != 0 {
		scale := t.Timescale
		if scale <= 0 {
			scale = int64(fps + 0.5)
		}
		return float64(t.PTS) / float64(scale), t.Timecode, nil
	}
	if strings.TrimSpace(t.Timecode) != "" {
		secs, err := parseTimecode(t.Timecode, fps)
		return secs, t.Timecode, err
	}
	frame := t.Frame
	if frame == 0 && t.Index != 0 {
		frame = t.Index
	}
	return float64(frame) / fps, formatTimecode(float64(frame)/fps, fps), nil
}

func (m Metadata) sequenceValue() int64 {
	if m.Sequence != 0 {
		return m.Sequence
	}
	return m.SequenceNumber
}

func (m Metadata) frameValue() (int64, bool) {
	if m.Frame != nil {
		return *m.Frame, true
	}
	if m.FrameIndex != nil {
		return *m.FrameIndex, true
	}
	if m.SequenceFrameIndex != nil {
		return *m.SequenceFrameIndex, true
	}
	return 0, false
}

func (m Metadata) sceneValue() *int64 {
	if m.SceneID != nil {
		return m.SceneID
	}
	return m.SceneNumber
}

func (m Metadata) versionValue() string {
	if m.Version != "" {
		return m.Version
	}
	return m.ParameterVersion
}

func (m Metadata) effectiveTime(fps float64, fallbackFrame int64) (float64, error) {
	if m.EffectiveSeconds != nil {
		return *m.EffectiveSeconds, nil
	}
	if m.EffectivePTS != nil {
		scale := m.PTSTimescale
		if scale <= 0 {
			scale = int64(fps + 0.5)
		}
		return float64(*m.EffectivePTS) / float64(scale), nil
	}
	if strings.TrimSpace(m.Timecode) != "" {
		return parseTimecode(m.Timecode, fps)
	}
	if m.Effective != nil {
		ref := m.Effective
		if ref.Seconds != 0 {
			return ref.Seconds, nil
		}
		if ref.PTS != 0 {
			scale := ref.Timescale
			if scale <= 0 {
				scale = int64(fps + 0.5)
			}
			return float64(ref.PTS) / float64(scale), nil
		}
		if strings.TrimSpace(ref.Timecode) != "" {
			return parseTimecode(ref.Timecode, fps)
		}
		if ref.Frame != nil {
			return float64(*ref.Frame) / fps, nil
		}
	}
	if frame, ok := m.frameValue(); ok {
		return float64(frame) / fps, nil
	}
	return float64(fallbackFrame) / fps, nil
}
