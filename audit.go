package hdrframe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type resolvedRecord struct {
	meta        Metadata
	index       int
	effective   float64
	version     string
	fingerprint string
}

// Audit validates and resolves every target frame in input.
func Audit(in Input) (*Report, error) {
	targets := in.Targets
	if len(targets) == 0 {
		targets = in.Frames
	}
	recordsIn := in.Metadata
	if len(recordsIn) == 0 {
		recordsIn = in.Records
	}
	events := in.PlayerEvents
	if len(events) == 0 {
		events = in.PlayerLog
	}
	if len(targets) == 0 {
		return nil, errors.New("hdrframe: at least one target/frame is required")
	}

	fpsText := in.FPS
	if strings.TrimSpace(fpsText) == "" {
		fpsText = "24"
	}
	fps, err := parseFPS(fpsText)
	if err != nil {
		return nil, err
	}
	policy := in.Policy.normalized()
	if in.Policy.TimeToleranceSecs == 0 {
		// Tolerate PTS/timecode rounding, but never bleed across a full frame.
		policy.TimeToleranceSecs = 0.25 / fps
	}

	rootIssues := make([]Issue, 0)
	records, resolveIssues := resolveRecords(recordsIn, fps)
	rootIssues = append(rootIssues, resolveIssues...)
	expectedIn := in.ExpectedMetadata
	if len(expectedIn) == 0 {
		expectedIn = in.SourceMetadata
	}
	if len(expectedIn) == 0 {
		expectedIn = in.ExpectedRecords
	}
	expectedRecords, expectedIssues := resolveRecords(expectedIn, fps)
	rootIssues = append(rootIssues, expectedIssues...)

	sort.SliceStable(targets, func(i, j int) bool {
		ti, _, _ := targetTime(targets[i], fps)
		tj, _, _ := targetTime(targets[j], fps)
		if ti == tj {
			return targets[i].Frame < targets[j].Frame
		}
		return ti < tj
	})

	report := &Report{
		SchemaVersion: 1, Name: in.Name, FPS: fpsText, FPSFloat: fps,
		Policy: policy, Diagnostics: rootIssues,
	}
	report.Duplicates = findDuplicates(records, policy)
	report.SequenceGaps = findSequenceGaps(records, policy)
	report.SceneChanges = findSceneChanges(records)

	eventByFrame := make(map[int64]PlayerEvent)
	for _, event := range events {
		frame := event.TargetFrame
		if frame == 0 {
			frame = event.Frame
		}
		eventByFrame[frame] = event
	}

	var lastApplied *resolvedRecord
	for _, target := range targets {
		frameReport := auditTarget(target, fps, policy, records, expectedRecords, report.Duplicates, report.SequenceGaps, eventByFrame[target.Frame], lastApplied)
		frameReport.flatten()
		if frameReport.Applied != nil {
			if rec := findRecordByApplied(frameReport.Applied, records); rec != nil {
				lastApplied = rec
			}
		}
		report.Frames = append(report.Frames, frameReport)
	}

	report.Summary = summarize(report)
	return report, nil
}

// Analyze is an alias for Audit for callers that prefer that name.
func Analyze(in Input) (*Report, error) { return Audit(in) }

// Resolve is a semantic alias for Audit.
func Resolve(in Input) (*Report, error) { return Audit(in) }

// Reconcile is a semantic alias for Audit.
func Reconcile(in Input) (*Report, error) { return Audit(in) }

// Verify is a semantic alias for Check.
func Verify(in Input) (*Report, error) { return Check(in) }

func resolveRecords(recordsIn []Metadata, fps float64) ([]resolvedRecord, []Issue) {
	records := make([]resolvedRecord, 0, len(recordsIn))
	var issues []Issue
	for i, meta := range recordsIn {
		effective, err := meta.effectiveTime(fps, 0)
		if err != nil {
			issues = append(issues, Issue{
				Code: "invalid_effective_time", Severity: "error",
				Message: err.Error(), Actual: meta.versionValue(),
			})
		}
		fp := meta.Checksum
		if fp == "" {
			fp = fingerprintMetadata(meta)
		}
		version := meta.versionValue()
		if version == "" {
			version = fallbackVersion(meta, i)
		}
		records = append(records, resolvedRecord{
			meta: meta, index: i, effective: effective,
			version: version, fingerprint: fp,
		})
	}
	return records, issues
}

// Check audits input and returns the report plus a non-nil error when any
// target is confirmed or inferred to apply the wrong parameter version.
func Check(in Input) (*Report, error) {
	report, err := Audit(in)
	if err != nil {
		return nil, err
	}
	if report.Summary.Mismatched > 0 {
		return report, fmt.Errorf("hdrframe: %d frame(s) apply an unexpected metadata version", report.Summary.Mismatched)
	}
	return report, nil
}

func (p Policy) normalized() Policy {
	out := p
	if out.Duplicate == "" {
		out.Duplicate = "latest"
	}
	if out.MissingMetadata == "" {
		out.MissingMetadata = "hold_last"
	}
	if out.SequenceGap == "" {
		out.SequenceGap = "accept"
	}
	if out.TimeToleranceSecs == 0 {
		out.TimeToleranceSecs = 1e-6
	}
	return out
}

func fingerprintMetadata(m Metadata) string {
	payload := struct {
		Sequence        int64           `json:"sequence,omitempty"`
		Frame           *int64          `json:"frame,omitempty"`
		SceneID         *int64          `json:"scene_id,omitempty"`
		SceneFrameIndex *int64          `json:"scene_frame_index,omitempty"`
		Highlight       HighlightParams `json:"highlight"`
		Color           ColorParams     `json:"color"`
		Values          map[string]any  `json:"values,omitempty"`
	}{
		Sequence: m.sequenceValue(), Frame: m.Frame, SceneID: m.sceneValue(),
		SceneFrameIndex: m.SceneFrameIndex, Highlight: m.Highlight,
		Color: m.Color, Values: m.Values,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

func fallbackVersion(m Metadata, index int) string {
	seq := m.sequenceValue()
	if seq != 0 {
		return fmt.Sprintf("seq-%d", seq)
	}
	if frame, ok := m.frameValue(); ok {
		return fmt.Sprintf("frame-%d-record-%d", frame, index)
	}
	return fmt.Sprintf("record-%d", index)
}

func recordKey(r resolvedRecord) string {
	seq := r.meta.sequenceValue()
	if seq != 0 {
		return fmt.Sprintf("seq:%d", seq)
	}
	if frame, ok := r.meta.frameValue(); ok {
		return fmt.Sprintf("frame:%d", frame)
	}
	return fmt.Sprintf("record:%d", r.index)
}

func findRecordByVersion(version string, records []resolvedRecord) *resolvedRecord {
	if version == "" {
		return nil
	}
	for i := range records {
		r := &records[i]
		if r.version == version || r.meta.ID == version || r.meta.ParameterVersion == version {
			return r
		}
	}
	return nil
}

func findRecordByApplied(applied *AppliedRef, records []resolvedRecord) *resolvedRecord {
	if applied == nil {
		return nil
	}
	if r := findRecordByVersion(applied.Version, records); r != nil {
		return r
	}
	for i := range records {
		r := &records[i]
		if applied.Checksum != "" && r.fingerprint == applied.Checksum {
			return r
		}
	}
	if applied.Sequence != 0 {
		var match *resolvedRecord
		count := 0
		for i := range records {
			r := &records[i]
			if r.meta.sequenceValue() == applied.Sequence {
				match = r
				count++
			}
		}
		if count == 1 {
			return match
		}
	}
	return nil
}
