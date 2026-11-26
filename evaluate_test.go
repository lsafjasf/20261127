package laddercheck

import (
	"encoding/json"
	"os"
	"testing"
)

func demoFacts(t *testing.T) []RenditionFacts {
	t.Helper()
	var facts []RenditionFacts
	for _, spec := range DemoSpecs() {
		f := parseSpecBytes(t, spec)
		f.ID = spec.ID
		f.Role = spec.Role
		facts = append(facts, f)
	}
	return facts
}

func statusByID(rep *Report) map[string]string {
	m := map[string]string{}
	for _, v := range rep.Verdicts {
		m[v.ID] = v.Status
	}
	return m
}

func TestDemoClassification(t *testing.T) {
	rep := EvaluateLadder("test", demoFacts(t), DemoConstraint())
	got := statusByID(rep)
	want := map[string]string{
		"rung-1080p": StatusCompliant,
		"rung-720p":  StatusRemuxOnly,
		"rung-480p":  StatusRemuxOnly,
		"rung-360p":  StatusReencode,
		"rung-240p":  StatusReencode,
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %s, want %s", id, got[id], w)
		}
	}
	if rep.Summary.SwitchSafe {
		t.Error("incident ladder must not be switch-safe")
	}
	if len(rep.Summary.RequiresReencode) != 2 {
		t.Errorf("requires_reencode=%v, want two rungs", rep.Summary.RequiresReencode)
	}
	if len(rep.Summary.RemuxOnly) != 2 {
		t.Errorf("remux_only=%v, want two rungs", rep.Summary.RemuxOnly)
	}
}

// TestRemuxOnlyCorrectionProof demonstrates that the remux-only rungs become
// compliant after a metadata-only edit-list/PTS correction (simulated by
// re-muxing the same coded samples with normalized labels).
func TestRemuxOnlyCorrectionProof(t *testing.T) {
	specs := DemoSpecs()

	// 720p: drop the empty lead edit.
	fixed720 := specs[1]
	fixed720.Edits = nil
	rep := EvaluateLadder("fix720", []RenditionFacts{withID(parseSpecBytes(t, fixed720), "720-fixed")}, DemoConstraint())
	if rep.Verdicts[0].Status != StatusCompliant {
		t.Fatalf("720p after elst normalization = %s: %v", rep.Verdicts[0].Status, rep.Verdicts[0].Reasons)
	}

	// 480p: keep the 0.1s CTO but add an identity-at-media-0.1 edit list.
	fixed480 := specs[2]
	fixed480.Edits = []FixtureEdit{{SegmentDurationTicks: 5000, MediaTimeTicks: 1536}}
	rep = EvaluateLadder("fix480", []RenditionFacts{withID(parseSpecBytes(t, fixed480), "480-elst")}, DemoConstraint())
	if rep.Verdicts[0].Status != StatusCompliant {
		t.Fatalf("480p after elst media_time correction = %s: %v", rep.Verdicts[0].Status, rep.Verdicts[0].Reasons)
	}

	// 360p cannot be helped by labels; only a 60-frame GOP re-encode fixes it.
	reenc360 := specs[3]
	reenc360.KeyframeSamples = []int{0, 60, 120}
	rep = EvaluateLadder("reenc360", []RenditionFacts{withID(parseSpecBytes(t, reenc360), "360-reenc")}, DemoConstraint())
	if rep.Verdicts[0].Status != StatusCompliant {
		t.Fatalf("360p after true re-encode = %s", rep.Verdicts[0].Status)
	}
}

func withID(f RenditionFacts, id string) RenditionFacts { f.ID = id; return f }

func TestEvidenceManifests(t *testing.T) {
	for _, tc := range []struct {
		path       string
		switchSafe bool
		reencode   int
	}{
		{"examples/ladder.evidence.json", false, 2},
		{"examples/ladder.aligned.json", true, 0},
	} {
		raw, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("read %s: %v", tc.path, err)
		}
		var doc LadderInput
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.path, err)
		}
		facts, opts, err := LoadLadder(&doc, nil)
		if err != nil {
			t.Fatalf("load %s: %v", tc.path, err)
		}
		rep := EvaluateLadder(tc.path, facts, opts)
		if rep.Summary.SwitchSafe != tc.switchSafe {
			t.Errorf("%s switchSafe=%v want %v (statuses=%v)", tc.path, rep.Summary.SwitchSafe, tc.switchSafe, statusByID(rep))
		}
		if len(rep.Summary.RequiresReencode) != tc.reencode {
			t.Errorf("%s reencode=%d want %d", tc.path, len(rep.Summary.RequiresReencode), tc.reencode)
		}
	}
}

func TestConstraintInference(t *testing.T) {
	facts := demoFacts(t)
	c := GenerateConstraint(facts, ConstraintOptions{})
	if c.GOPSeconds.Cmp(NewRat(2, 1)) != 0 {
		t.Fatalf("inferred gop=%s want 2s", c.GOPSeconds)
	}
	if c.GOPFrames != 60 {
		t.Fatalf("inferred gop frames=%d want 60", c.GOPFrames)
	}
	if c.FPS.Cmp(NewRat(30, 1)) != 0 {
		t.Fatalf("inferred fps=%s", c.FPS)
	}
}
