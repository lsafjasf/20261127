package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// InferStage guesses the pipeline stage from a file name/path.
func InferStage(path string) Stage {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case containsAny(base, "player", "playback", "render", "present", "播放"):
		return StagePlayer
	case containsAny(base, "captur", "cam", "source", "采集"):
		return StageCapture
	case containsAny(base, "record", "encod", "mux", "输出", "录制", "录像", "编码"):
		return StageRecord
	}
	return StageUnknown
}

// LoadTraceFile loads one CSV/TSV/JSON/JSONL frame trace.
func LoadTraceFile(path string, stage Stage) (*Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jsonl", ".ndjson":
		return loadJSONL(f, path, stage)
	case ".json":
		return loadJSONFile(f, path, stage)
	default:
		return loadDelimited(f, path, stage)
	}
}

func loadDelimited(r io.Reader, path string, stage Stage) (*Trace, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	comma := sniffComma(string(data))
	cr := csv.NewReader(strings.NewReader(string(data)))
	cr.Comma = comma
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv %s: %w", path, err)
	}
	if len(rows) == 0 {
		return &Trace{Stage: stage, Source: path}, nil
	}
	header := rows[0]
	idx := map[string]int{}
	for i, h := range header {
		idx[normalizeKey(h)] = i
	}
	t := &Trace{Stage: stage, Source: path}
	for ri := 1; ri < len(rows); ri++ {
		row := rows[ri]
		if len(row) == 1 && strings.TrimSpace(row[0]) == "" {
			continue
		}
		m := map[string]string{}
		for key, ci := range idx {
			if ci < len(row) {
				m[key] = row[ci]
			}
		}
		fr, _ := buildFrame(m, ri+1, stage)
		t.addRow(fr)
	}
	return t, nil
}

func sniffComma(head string) rune {
	line := head
	if i := strings.IndexByte(head, '\n'); i >= 0 {
		line = head[:i]
	}
	counts := map[rune]int{'\t': strings.Count(line, "\t"), ';': strings.Count(line, ";"), ',': strings.Count(line, ",")}
	best, bestN := ',', counts[',']
	for _, c := range []rune{'\t', ';'} {
		if counts[c] > bestN {
			best, bestN = c, counts[c]
		}
	}
	return best
}

func loadJSONL(r io.Reader, path string, stage Stage) (*Trace, error) {
	dec := json.NewDecoder(r)
	t := &Trace{Stage: stage, Source: path}
	line := 0
	for dec.More() {
		line++
		var raw map[string]json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("parse jsonl %s line %d: %w", path, line, err)
		}
		m := rawToStrings(raw)
		fr, _ := buildFrame(m, line, stage)
		t.addRow(fr)
	}
	return t, nil
}

func loadJSONFile(r io.Reader, path string, stage Stage) (*Trace, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	t := &Trace{Stage: stage, Source: path}
	if strings.HasPrefix(trimmed, "[") {
		var raws []map[string]json.RawMessage
		if err := json.Unmarshal(data, &raws); err != nil {
			return nil, fmt.Errorf("parse json %s: %w", path, err)
		}
		for i, raw := range raws {
			fr, _ := buildFrame(rawToStrings(raw), i+1, stage)
			t.addRow(fr)
		}
		return t, nil
	}
	var envelope struct {
		Frames  []map[string]json.RawMessage `json:"frames"`
		Records []map[string]json.RawMessage `json:"records"`
		Data    []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil {
		raws := envelope.Frames
		if len(raws) == 0 {
			raws = envelope.Records
		}
		if len(raws) == 0 {
			raws = envelope.Data
		}
		for i, raw := range raws {
			fr, _ := buildFrame(rawToStrings(raw), i+1, stage)
			t.addRow(fr)
		}
		return t, nil
	}
	// Fall back to JSONL.
	return loadJSONL(strings.NewReader(trimmed), path, stage)
}

func rawToStrings(raw map[string]json.RawMessage) map[string]string {
	m := map[string]string{}
	for k, v := range raw {
		key := normalizeKey(k)
		s := string(v)
		if len(s) > 0 && s[0] == '"' {
			var str string
			if err := json.Unmarshal(v, &str); err == nil {
				s = str
			}
		} else {
			s = strings.TrimSpace(s)
		}
		m[key] = s
	}
	return m
}

func (t *Trace) addRow(f *Frame) {
	if f.HasDropped && f.Dropped {
		t.Markers = append(t.Markers, f)
		return
	}
	t.Frames = append(t.Frames, f)
}

var traceExts = map[string]bool{".csv": true, ".tsv": true, ".txt": true, ".json": true, ".jsonl": true, ".ndjson": true}

// LoadDir scans a directory for trace files and assigns stages by file name.
func LoadDir(dir string) (Sources, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Sources{}, nil, err
	}
	var warnings []string
	src := Sources{}
	for _, e := range entries {
		if e.IsDir() || !traceExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		p := filepath.Join(dir, e.Name())
		st := InferStage(e.Name())
		if st == StageUnknown {
			tr, err := LoadTraceFile(p, StageRecord)
			if err != nil || len(tr.Frames) == 0 {
				continue // not a trace (e.g. scenario.json) or unreadable
			}
			if src.Record == nil {
				src.Record = tr
			} else {
				warnings = append(warnings, "多个无法识别阶段的轨迹，已跳过: "+p)
			}
			continue
		}
		tr, err := LoadTraceFile(p, st)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		if len(tr.Frames) == 0 && len(tr.Markers) == 0 {
			warnings = append(warnings, "轨迹为空，已跳过: "+p)
			continue
		}
		assignTrace(&src, tr)
	}
	return src, warnings, nil
}

func assignTrace(src *Sources, t *Trace) {
	switch t.Stage {
	case StageCapture:
		src.Capture = t
	case StagePlayer:
		src.Player = t
	default:
		t.Stage = StageRecord
		src.Record = t
	}
}

// splitByStage fans a single file with an explicit stage column into three
// traces. It returns nil when all rows belong to one stage.
func splitByStage(tr *Trace) *Sources {
	buckets := map[Stage]*Trace{}
	get := func(st Stage) *Trace {
		if b, ok := buckets[st]; ok {
			return b
		}
		b := &Trace{Stage: st, Source: tr.Source}
		buckets[st] = b
		return b
	}
	distinct := map[Stage]bool{}
	for _, f := range tr.Frames {
		distinct[f.Stage] = true
		get(f.Stage).Frames = append(get(f.Stage).Frames, f)
	}
	for _, m := range tr.Markers {
		distinct[m.Stage] = true
		get(m.Stage).Markers = append(get(m.Stage).Markers, m)
	}
	if len(distinct) < 2 {
		return nil
	}
	src := &Sources{}
	for _, b := range buckets {
		assignTrace(src, b)
	}
	return src
}
