package check

import "testing"

func TestAggregateStatusFailWinsAfterEarlierWarning(t *testing.T) {
	got := aggregateStatus([]Status{StatusWarn, StatusFail, StatusPass})
	if got != StatusFail {
		t.Fatalf("aggregate = %q, want fail", got)
	}
}

func TestTrustedPixelPassBeatsUnattestedGoodPacketRecovery(t *testing.T) {
	bundle := Bundle{
		Observations: trustedPasses("f0004", 4),
		Recovery: []Recovery{{
			FrameKey:        "f0004",
			GoodPacket:      true,
			BitExactDecoder: false,
		}},
	}

	report, err := Analyze(bundle)
	if err != nil {
		t.Fatal(err)
	}
	verdict := report.FrameVerdicts[0]
	if verdict.Reconstructable != ReconstructYes {
		t.Fatalf("reconstructable = %q, want yes", verdict.Reconstructable)
	}
	if verdict.Mode != "preserve_already_verified_pixels" {
		t.Fatalf("mode = %q", verdict.Mode)
	}
}

func TestDownstreamCRCPassDoesNotClearAncestorFailure(t *testing.T) {
	downstream := trustedPasses("f0002", 2)
	for i := range downstream {
		if downstream[i].Layer == LayerPixels {
			downstream[i].TrustedReference = false
			downstream[i].Status = StatusUnknown
		}
		downstream[i].Frame.References = []string{"f0001"}
	}
	bundle := Bundle{Observations: append([]Observation{
		{Layer: LayerBitstream, Status: StatusFail, Check: "crc", Frame: FrameRef{Key: "f0001", DecodeOrder: 1}},
		{Layer: LayerPixels, Status: StatusFail, TrustedReference: true, Check: "pixel", Frame: FrameRef{Key: "f0001", DecodeOrder: 1}},
	}, downstream...)}

	report, err := Analyze(bundle)
	if err != nil {
		t.Fatal(err)
	}
	verdict := verdictsByKey(report.FrameVerdicts)["f0002"]
	if len(verdict.ImpactedBy) != 1 || verdict.ImpactedBy[0] != "f0001" {
		t.Fatalf("impacted_by = %#v, want [f0001]", verdict.ImpactedBy)
	}
	if verdict.Reconstructable != ReconstructUnknown {
		t.Fatalf("reconstructable = %q, want unknown", verdict.Reconstructable)
	}
	if len(verdict.Warnings) == 0 {
		t.Fatal("expected reference propagation warning")
	}
}
