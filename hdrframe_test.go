package hdrframe

import (
	"path/filepath"
	"testing"
)

func reportFrame(t *testing.T, report *Report, frame int64) FrameReport {
	t.Helper()
	for _, item := range report.Frames {
		if item.Frame == frame {
			return item
		}
	}
	t.Fatalf("frame %d not found", frame)
	return FrameReport{}
}

func hasCode(frame FrameReport, code string) bool {
	for _, issue := range frame.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestDuplicateSampleIdentifiesActuallyAppliedOldVersion(t *testing.T) {
	report, err := Audit(DuplicateSample())
	if err != nil {
		t.Fatal(err)
	}
	frame := reportFrame(t, report, 122)
	if frame.Status != "mismatch" {
		t.Fatalf("status=%s, want mismatch", frame.Status)
	}
	if frame.Expected == nil || frame.Expected.Version != "HM-122-B-right" {
		t.Fatalf("expected version=%+v", frame.Expected)
	}
	if frame.Selected == nil || frame.Selected.Version != "HM-122-B-right" {
		t.Fatalf("selected version=%+v, want latest duplicate B", frame.Selected)
	}
	if frame.Applied == nil || frame.Applied.Version != "HM-122-A-old" {
		t.Fatalf("applied=%+v, want player-used A", frame.Applied)
	}
	if !frame.Applied.Verified {
		t.Fatal("player log should make applied version verified")
	}
	if len(frame.ParameterDiffs) < 3 {
		t.Fatalf("parameter diffs=%+v, want brightness/gain/hue deltas", frame.ParameterDiffs)
	}
	if len(report.Duplicates) != 1 || report.Duplicates[0].Identical {
		t.Fatalf("duplicates=%+v, want one conflict", report.Duplicates)
	}
}

func TestMissingFrameSampleShowsStaleHueAndHighlightVersion(t *testing.T) {
	report, err := Audit(MissingFrameSample())
	if err != nil {
		t.Fatal(err)
	}
	frame := reportFrame(t, report, 202)
	if frame.Status != "mismatch" {
		t.Fatalf("status=%s, want mismatch", frame.Status)
	}
	if frame.Applied == nil || frame.Applied.Version != "SRC-201" {
		t.Fatalf("applied=%+v, want held SRC-201", frame.Applied)
	}
	if !frame.Applied.MatchesSelected || frame.Applied.MatchesExpected {
		t.Fatalf("match flags selected=%v expected=%v", frame.Applied.MatchesSelected, frame.Applied.MatchesExpected)
	}
	if frame.Expected == nil || frame.Expected.Version != "HM-202-required" {
		t.Fatalf("expected=%+v", frame.Expected)
	}
	if !hasCode(frame, "missing_exact_frame_metadata") || !hasCode(frame, "stale_or_wrong_metadata_applied") {
		t.Fatalf("issues=%+v", frame.Issues)
	}
	if len(frame.ParameterDiffs) < 3 {
		t.Fatalf("missing-frame parameter diffs=%+v", frame.ParameterDiffs)
	}
}

func TestSequenceJumpIsReportedButFramePTSRemainsAuthoritative(t *testing.T) {
	report, err := Audit(SequenceJumpSample())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SequenceGaps) != 1 {
		t.Fatalf("gaps=%+v", report.SequenceGaps)
	}
	gap := report.SequenceGaps[0]
	if gap.AfterSequence != 301 || gap.BeforeSequence != 305 || gap.MissingCount != 3 {
		t.Fatalf("unexpected gap: %+v", gap)
	}
	frame := reportFrame(t, report, 302)
	if frame.Status != "matched" {
		t.Fatalf("status=%s issues=%+v", frame.Status, frame.Issues)
	}
	if frame.Applied == nil || frame.Applied.Sequence != 305 || !frame.Applied.Verified {
		t.Fatalf("applied=%+v, want verified sequence 305", frame.Applied)
	}
	if !hasCode(frame, "sequence_jump") {
		t.Fatalf("missing sequence_jump issue: %+v", frame.Issues)
	}
}

func TestInferredHoldLastWithoutPlayerLog(t *testing.T) {
	scene := Int64Ptr(9)
	input := Input{
		FPS:    "10",
		Policy: Policy{MissingMetadata: "hold_last"},
		Targets: []Target{
			{Frame: 1, ExpectedSceneID: scene},
			{Frame: 2, ExpectedSceneID: scene},
			{Frame: 3, ExpectedSceneID: scene},
		},
		Metadata: []Metadata{
			sampleMeta("v1", 1, scene, 0, 1, 1000, 1, 0),
			sampleMeta("v3", 3, scene, 2, 3, 2000, 2, -5),
		},
	}
	input.Metadata = withFPS(input.Metadata, 10)
	report, err := Audit(input)
	if err != nil {
		t.Fatal(err)
	}
	frame := reportFrame(t, report, 2)
	if frame.Applied == nil || frame.Applied.Version != "v1" || frame.Applied.Verified {
		t.Fatalf("applied=%+v, want inferred unverified v1", frame.Applied)
	}
	if frame.Status != "matched" {
		t.Fatalf("status=%s, inferred hold should match delivered timeline", frame.Status)
	}
	if report.Summary.Unverified != 3 {
		t.Fatalf("unverified=%d, want all frames without direct player evidence", report.Summary.Unverified)
	}
}

func TestEffectiveTimeGatesFutureRecord(t *testing.T) {
	input := Input{
		FPS:     "10",
		Targets: []Target{{Frame: 1, PTSSeconds: 0.1}, {Frame: 2, PTSSeconds: 0.2}},
		Metadata: []Metadata{{
			Version: "future-v2", Sequence: 2,
			EffectiveSeconds: Float64Ptr(0.2),
			Highlight:        HighlightParams{PeakNits: 4000},
		}},
	}
	report, err := Audit(input)
	if err != nil {
		t.Fatal(err)
	}
	first := reportFrame(t, report, 1)
	if first.Selected != nil {
		t.Fatalf("future metadata must not be selected at 0.1s: %+v", first.Selected)
	}
	second := reportFrame(t, report, 2)
	if second.Selected == nil || second.Selected.Version != "future-v2" {
		t.Fatalf("at effective time selection=%+v", second.Selected)
	}
}

func TestWriteSampleDirAndReload(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSampleDir(dir); err != nil {
		t.Fatal(err)
	}
	file, err := LoadSampleFile(filepath.Join(dir, "samples.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Samples) != 3 {
		t.Fatalf("samples=%d, want 3", len(file.Samples))
	}
	batch, err := AuditInputs(file.Samples...)
	if err != nil {
		t.Fatal(err)
	}
	if !batch.HasMismatch() {
		t.Fatal("duplicate and missing fixtures should contain mismatches")
	}
}
