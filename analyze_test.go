package hdrswitch

import (
	"strings"
	"testing"
)

func TestRationalComparisonUsesCrossProducts(t *testing.T) {
	later := MustRational(17999, 30)
	cut := MustRational(600, 1)
	if later.Cmp(cut) >= 0 {
		t.Fatalf("17999/30 = %v must be before 600", later.Float64())
	}
	if MustRational(18000, 30).Cmp(cut) != 0 {
		t.Fatal("18000/30 must equal 600")
	}
}

func TestRiskySampleReportsGOPBeforeRanges(t *testing.T) {
	report, err := Analyze(SampleEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatal("risky sample must not be OK")
	}
	if report.Classification != "unsafe_gop_switch" {
		t.Fatalf("classification = %q", report.Classification)
	}
	if report.GOPRisk == nil || !report.GOPRisk.AtRisk || report.GOPRisk.Safe {
		t.Fatalf("unexpected GOP risk: %+v", report.GOPRisk)
	}
	if report.GOPRisk.CutAUIndex != 18000 || report.GOPRisk.PositionFromGOPStart != 60 {
		t.Fatalf("cut/offset = %d/%d", report.GOPRisk.CutAUIndex, report.GOPRisk.PositionFromGOPStart)
	}
	if report.GOPRisk.GOPStartAUIndex == nil || *report.GOPRisk.GOPStartAUIndex != 17940 {
		t.Fatalf("GOP start = %+v", report.GOPRisk.GOPStartAUIndex)
	}
	if report.GOPRisk.PreviousRandomAccess == nil || report.GOPRisk.PreviousRandomAccess.AUIndex != 17940 {
		t.Fatal("missing previous random access boundary")
	}
	if report.GOPRisk.NextRandomAccess == nil || report.GOPRisk.NextRandomAccess.AUIndex != 18060 {
		t.Fatal("missing next random access boundary")
	}

	before := report.Before.LegalRange
	after := report.After.LegalRange
	if before.Transfer != TransferHLG || before.H273TransferCode != 18 {
		t.Fatalf("before transfer = %s/%d", before.Transfer, before.H273TransferCode)
	}
	if after.Transfer != TransferPQ || after.H273TransferCode != 16 {
		t.Fatalf("after transfer = %s/%d", after.Transfer, after.H273TransferCode)
	}
	for _, envelope := range []LegalEnvelope{before, after} {
		if envelope.ComponentCodes.Luma.Min != 64 || envelope.ComponentCodes.Luma.Max != 940 {
			t.Fatalf("luma legal codes = %+v", envelope.ComponentCodes.Luma)
		}
		if envelope.ComponentCodes.Chroma.Min != 64 || envelope.ComponentCodes.Chroma.Max != 960 {
			t.Fatalf("chroma legal codes = %+v", envelope.ComponentCodes.Chroma)
		}
		if envelope.Gamut.Primaries != PrimariesBT2020 || envelope.Gamut.PrimariesCode != 9 {
			t.Fatalf("gamut = %+v", envelope.Gamut)
		}
	}
	if before.SceneLinearSignal == nil || before.ReferenceLuminance == nil || before.ReferenceLuminance.Max != 1000 {
		t.Fatalf("HLG range = scene=%+v ref=%+v", before.SceneLinearSignal, before.ReferenceLuminance)
	}
	if after.RepresentationLuminance == nil || after.RepresentationLuminance.Max != 10000 {
		t.Fatalf("PQ representation = %+v", after.RepresentationLuminance)
	}
	if after.ReferenceLuminance == nil || after.ReferenceLuminance.Max != 4000 {
		t.Fatalf("PQ declared reference = %+v", after.ReferenceLuminance)
	}
	if report.Player == nil || report.Player.MismatchAUCount != 3 || *report.Player.FirstMismatchAU != 18000 {
		t.Fatalf("player report = %+v", report.Player)
	}
	if !hasCode(report.Findings, "switch_inside_gop") || !hasCode(report.Findings, "player_sticky_first_frame_metadata") {
		t.Fatalf("findings = %+v", report.Findings)
	}
	if report.Findings[0].Code != "switch_inside_gop" {
		t.Fatalf("first finding must be GOP risk, got %s", report.Findings[0].Code)
	}
}

func TestSafeIDRSampleIsGOPSafeButPlayerStickyFails(t *testing.T) {
	report, err := Analyze(SafeSampleEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if report.GOPRisk == nil || report.GOPRisk.AtRisk || !report.GOPRisk.Safe {
		t.Fatalf("safe sample GOP = %+v", report.GOPRisk)
	}
	if !hasCode(report.Findings, "safe_au_boundary") {
		t.Fatal("missing safe boundary finding")
	}
	if report.OK {
		t.Fatal("sticky player should still make safe GOP sample fail check")
	}

	evidence := SafeSampleEvidence()
	evidence.Player.UsesFirstFrameMetadata = false
	report, err = Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("correct player should pass: %+v", report.Findings)
	}

	validReport, err := Analyze(ValidSampleEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if !validReport.OK || validReport.Classification != "valid_au_aligned_switch" {
		t.Fatalf("valid sample = ok=%v class=%s findings=%+v", validReport.OK, validReport.Classification, validReport.Findings)
	}
}

func TestNonAUAlignedSwitchIsReported(t *testing.T) {
	evidence := SampleEvidence()
	evidence.SwitchAt = MustRational(60001, 100)
	report, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.CutAlignment != "between_access_units" {
		t.Fatalf("alignment = %s", report.CutAlignment)
	}
	if !hasCode(report.Findings, "switch_not_au_aligned") {
		t.Fatalf("missing alignment finding: %+v", report.Findings)
	}
}

func TestGamutChangeAppearsInAfterEnvelope(t *testing.T) {
	evidence := SampleEvidence()
	for i := range evidence.AccessUnits {
		if evidence.AccessUnits[i].Index == 18000 {
			evidence.AccessUnits[i].Metadata.Primaries = PrimariesP3D65
			evidence.AccessUnits[i].Metadata.PrimariesCode = 12
		}
	}
	report, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.Before.LegalRange.Gamut.Primaries != PrimariesBT2020 {
		t.Fatalf("before gamut = %s", report.Before.LegalRange.Gamut.Primaries)
	}
	if report.After.LegalRange.Gamut.Primaries != PrimariesP3D65 {
		t.Fatalf("after gamut = %s", report.After.LegalRange.Gamut.Primaries)
	}
	if report.Player == nil || !containsPrefix(report.Player.StaleFields, "primaries:") {
		t.Fatalf("stale fields should mention gamut change: %+v", report.Player)
	}
}

func TestPlanRefusesGlobalOnlyShortcutInsideGOP(t *testing.T) {
	plan, report, err := PlanRepair(SampleEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || plan.SafeToRetag {
		t.Fatal("plan must not permit direct retag inside GOP")
	}
	if plan.Mode != "fix_gop_before_metadata_retag" {
		t.Fatalf("mode = %s", plan.Mode)
	}
	if plan.PreviousRandomAccess == nil || plan.NextRandomAccess == nil {
		t.Fatal("plan must include alternative RAP boundaries")
	}
	if len(plan.MetadataUpdates) != 2 {
		t.Fatalf("updates = %+v", plan.MetadataUpdates)
	}
}

func hasCode(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func containsPrefix(fields []string, prefix string) bool {
	for _, field := range fields {
		if strings.HasPrefix(field, prefix) {
			return true
		}
	}
	return false
}
