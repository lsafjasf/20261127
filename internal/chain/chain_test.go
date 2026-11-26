package chain

import (
	"strings"
	"testing"

	"evidencechain/internal/casegen"
	"evidencechain/internal/model"
)

func buildFixtureCase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := casegen.Generate(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildIndex(dir, casegen.CaseID, 128); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFullChainClassification(t *testing.T) {
	dir := buildFixtureCase(t)
	v, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK {
		t.Fatalf("fresh chain should be OK, errors=%v", v.Errors)
	}

	count := map[string]int{}
	for _, f := range v.Files {
		count[f.Status]++
	}
	if count["identified"] != 2 {
		t.Fatalf("expected 2 identified renamed files, got %d (%+v)", count["identified"], v.Files)
	}
	if count["tampered-localized"] != 1 {
		t.Fatalf("expected 1 tampered file, got %d", count["tampered-localized"])
	}
	if count["similar-only"] != 1 || count["unknown"] != 1 {
		t.Fatalf("expected 1 similar-only and 1 unknown, got %v", count)
	}

	var tampered *model.FileResult
	for i := range v.Files {
		if v.Files[i].Status == "tampered-localized" {
			tampered = &v.Files[i]
		}
	}
	if tampered == nil || len(tampered.BlockDiffs) == 0 || tampered.MatchedBlocks == 0 {
		t.Fatal("tampered file must carry block localization with matches and diffs")
	}

	cstat := map[string]int{}
	for _, c := range v.Containers {
		cstat[c.Status]++
	}
	if cstat["pristine-anchor-set"] != 2 || cstat["pristine-subset"] != 1 || cstat["contains-altered-or-unknown"] != 1 {
		t.Fatalf("unexpected container statuses: %v", cstat)
	}

	foundRejected := false
	for _, w := range v.Watchlist {
		if w.Status == "rejected-prefix-only" {
			foundRejected = true
		}
	}
	if !foundRejected {
		t.Fatal("lookalike prefix hash must be rejected, not matched")
	}

	for _, l := range v.Links {
		if l.Status != "verified" {
			t.Fatalf("provenance link %s not verified: %s", l.ID, l.Detail)
		}
	}
}

func TestOverclaimJudgmentFails(t *testing.T) {
	dir := buildFixtureCase(t)
	bad := model.Judgments{
		Schema: model.Schema, CaseID: casegen.CaseID, Analyst: "t",
		Items: []model.Judgment{{
			ID: "BX-1", Target: "file:30-field-returns/设备缓存_dc88.bin",
			Verdict: "identity-confirmed", Confidence: "高",
			Basis:     []string{"fuzzy-similarity"},
			Rationale: "a reviewer wrongly treats similarity as identity",
		}},
	}
	if err := WriteJSON(dir+"/review/judgments.json", bad); err != nil {
		t.Fatal(err)
	}
	v, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v.OK {
		t.Fatal("overclaim judgment must make verification fail")
	}
	saw := false
	for _, j := range v.Judgments {
		if j.ID == "BX-1" && (j.Status == "overclaim" || j.Status == "contradiction") {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected BX-1 overclaim/contradiction, got %+v", v.Judgments)
	}
}

func TestRenderReportContainsPolicy(t *testing.T) {
	dir := buildFixtureCase(t)
	md, v, err := RenderReport(dir, "case/")
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK {
		t.Fatalf("report verification should pass: %v", v.Errors)
	}
	for _, want := range []string{"电子数据取证检验报告", "相似哈希不是同一证据", "uncertain-origin", "memberset_sha256", "rejected-prefix-only"} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing section/text %q", want)
		}
	}
}
