package tailtrace

import "testing"

func TestSampleLocatesTwoTailBlackFrames(t *testing.T) {
	report, err := Analyze(SampleEvidence())
	if err != nil {
		t.Fatal(err)
	}

	if report.Classification != "renderer_tail_black_padding_contract_violation" {
		t.Fatalf("classification = %q", report.Classification)
	}
	if report.OK {
		t.Fatal("sample should violate the clean delivery contract")
	}
	if report.Durations.OutputFrames != 12 || report.Durations.TailBlackFrames != 2 {
		t.Fatalf("frames = %+v", report.Durations)
	}
	if got := report.Durations.TailBlackStartPTS; got == nil || got.Cmp(MustRational(1, 3)) != 0 {
		t.Fatalf("first black PTS = %v, want 1/3", got)
	}
	if report.Durations.ContentDuration.Cmp(MustRational(1, 3)) != 0 {
		t.Fatalf("content duration = %s", report.Durations.ContentDuration)
	}
	if report.Durations.CFRDuration.Cmp(MustRational(2, 5)) != 0 {
		t.Fatalf("CFR duration = %s, want 2/5", report.Durations.CFRDuration)
	}
	if report.Durations.LastPTS.Cmp(MustRational(11, 30)) != 0 {
		t.Fatalf("last PTS = %s", report.Durations.LastPTS)
	}
	if report.Durations.LastPTSEnd.Cmp(MustRational(2, 5)) != 0 {
		t.Fatalf("last end = %s", report.Durations.LastPTSEnd)
	}
	if !hasFinding(report.Findings, "tail_black_padding_located") {
		t.Fatalf("missing location finding: %+v", report.Findings)
	}
	if !hasFinding(report.Findings, "tail_padding_aligns_with_audio_duration") {
		t.Fatalf("missing audio alignment finding: %+v", report.Findings)
	}
}

func TestTrimFixProducesCleanContract(t *testing.T) {
	evidence := SampleEvidence()
	plan, err := PlanTrim(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RemoveIndices) != 2 || plan.RemoveIndices[0] != 10 || plan.RemoveIndices[1] != 11 {
		t.Fatalf("remove = %v", plan.RemoveIndices)
	}

	fixed, err := Analyze(plan.FixedEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if !fixed.OK || fixed.Classification != "clean_cfr_output" {
		t.Fatalf("fixed report ok=%v class=%s findings=%+v", fixed.OK, fixed.Classification, fixed.Findings)
	}
	if fixed.Durations.OutputFrames != 10 || fixed.Durations.TailBlackFrames != 0 {
		t.Fatalf("fixed durations = %+v", fixed.Durations)
	}
	if fixed.Durations.CFRDuration.Cmp(MustRational(1, 3)) != 0 {
		t.Fatalf("fixed duration = %s", fixed.Durations.CFRDuration)
	}
}

func TestContractFixExplainsIntentionalPadding(t *testing.T) {
	evidence := SampleEvidence()
	plan, err := PlanContract(evidence)
	if err != nil {
		t.Fatal(err)
	}

	evidence.Contract = *plan.RevisedContract
	revised, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !revised.OK {
		t.Fatalf("revised contract should pass: %+v", revised.Findings)
	}
	if revised.Classification != "contract_allowed_tail_black_padding" {
		t.Fatalf("classification = %q", revised.Classification)
	}
	if revised.Durations.ContractFrames != 12 || revised.Durations.ContractDuration.Cmp(MustRational(2, 5)) != 0 {
		t.Fatalf("revised contract = %+v", revised.Durations)
	}
}

func TestNTSCDurationIsExact(t *testing.T) {
	evidence := SampleEvidence()
	fps := MustRational(30000, 1001)
	frameDuration := Rational{Num: fps.Den, Den: fps.Num}
	evidence.Contract.FPS = fps
	evidence.PTSTimeBase = nil
	for i := range evidence.Output {
		evidence.Output[i].PTSTicks = nil
		evidence.Output[i].TimeBase = nil
		pts := MustRational(int64(i), 1).Mul(frameDuration)
		evidence.Output[i].PTS = &pts
	}

	report, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	wantTail := MustRational(1001, 15000)
	if report.Durations.TailBlackDuration.Cmp(wantTail) != 0 {
		t.Fatalf("tail duration = %s, want %s", report.Durations.TailBlackDuration, wantTail)
	}
	wantEnd := MustRational(12*1001, 30000)
	if report.Durations.LastPTSEnd.Cmp(wantEnd) != 0 {
		t.Fatalf("end = %s, want %s", report.Durations.LastPTSEnd, wantEnd)
	}
}

func TestEditListCanHideTailFromCompliantPlayers(t *testing.T) {
	evidence := SampleEvidence()
	contentEnd := MustRational(1, 3)
	evidence.Contract.Player.EditListEnd = &contentEnd
	evidence.Contract.Player.ReportedDuration = &contentEnd

	report, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(report.Findings, "edit_list_trims_tail_for_players") {
		t.Fatalf("missing edit-list finding: %+v", report.Findings)
	}
	if hasFinding(report.Findings, "player_duration_mismatch") {
		t.Fatalf("player duration should match edit list: %+v", report.Findings)
	}
	if !hasFinding(report.Findings, "container_duration_differs_from_edit_list") {
		t.Fatalf("missing container/edit-list explanation: %+v", report.Findings)
	}
}

func TestPTSDriftIsNotMisclassifiedAsPadding(t *testing.T) {
	evidence := SampleEvidence()
	evidence.Output[11].PTSTicks = nil
	driftedPTS := MustRational(11, 30).Add(MustRational(1, 1000))
	evidence.Output[11].PTS = &driftedPTS

	report, err := Analyze(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.Classification != "pts_generation_error" || report.OK {
		t.Fatalf("class=%s ok=%v findings=%+v", report.Classification, report.OK, report.Findings)
	}
}
