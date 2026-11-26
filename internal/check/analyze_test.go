package check

import "testing"

func TestPromptPatternFindsContainerAnomalyFirstAndRejectsPixelReconstruction(t *testing.T) {
	bundle := Bundle{
		Observations: []Observation{},
	}
	bundle.Observations = append(bundle.Observations, Observation{Layer: LayerContainer, Status: StatusFail, Check: "mkvalidator", Message: "container metadata mismatch", Frame: FrameRef{Key: "f0003", DecodeOrder: 3}})
	bundle.Observations = append(bundle.Observations, trustedPasses("f0003", 3)...)
	bundle.Observations = append(bundle.Observations, Observation{Layer: LayerContainer, Status: StatusFail, Check: "ffprobe_packet_pts", Message: "pts/dts ordering anomaly", Frame: FrameRef{Key: "f0007", DecodeOrder: 7}})
	bundle.Observations = append(bundle.Observations, trustedPasses("f0007", 7)...)
	bundle.Observations = append(bundle.Observations,
		Observation{Layer: LayerContainer, Status: StatusPass, Check: "ffprobe", Frame: FrameRef{Key: "f0012", DecodeOrder: 12}},
		Observation{Layer: LayerBitstream, Status: StatusFail, Check: "slice_crc", Expected: "9f", Actual: "21", Frame: FrameRef{Key: "f0012", DecodeOrder: 12}},
		Observation{Layer: LayerDecode, Status: StatusWarn, Check: "decoder", Message: "concealed corrupted MBs", Frame: FrameRef{Key: "f0012", DecodeOrder: 12}},
		Observation{Layer: LayerPixels, Status: StatusFail, TrustedReference: true, Check: "sha256_framehash", Frame: FrameRef{Key: "f0012", DecodeOrder: 12}},
	)

	report, err := Analyze(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if report.Earliest == nil {
		t.Fatal("expected earliest failure")
	}
	if report.Earliest.FrameKey != "f0003" || report.Earliest.Layer != LayerContainer {
		t.Fatalf("earliest = %s/%s, want f0003/container", report.Earliest.FrameKey, report.Earliest.Layer)
	}
	if got := report.EarliestByLayer[LayerBitstream].FrameKey; got != "f0012" {
		t.Fatalf("bitstream earliest = %q, want f0012", got)
	}
	if got := report.EarliestByLayer[LayerPixels].FrameKey; got != "f0012" {
		t.Fatalf("pixel earliest = %q, want f0012", got)
	}

	byKey := verdictsByKey(report.FrameVerdicts)
	if got := byKey["f0003"].Reconstructable; got != ReconstructYes {
		t.Fatalf("container-only frame reconstruction = %q, want yes", got)
	}
	if got := byKey["f0003"].Mode; got != "preserve_already_verified_pixels" {
		t.Fatalf("container-only mode = %q", got)
	}
	if got := byKey["f0012"].Reconstructable; got != ReconstructNo {
		t.Fatalf("crc+pixel failure reconstruction = %q, want no_from_current_evidence", got)
	}
}

func TestCRCPassWithPixelFailureIsNotContentCorrect(t *testing.T) {
	bundle := Bundle{Observations: []Observation{
		{Layer: LayerBitstream, Status: StatusPass, Frame: FrameRef{Key: "f0002", DecodeOrder: 2}, Check: "stored_crc32"},
		{Layer: LayerPixels, Status: StatusFail, TrustedReference: true, Frame: FrameRef{Key: "f0002", DecodeOrder: 2}, Check: "pixel_hash"},
	}}

	report, err := Analyze(bundle)
	if err != nil {
		t.Fatal(err)
	}
	verdict := report.FrameVerdicts[0]
	if verdict.Reconstructable != ReconstructNo {
		t.Fatalf("reconstructable = %q, want no", verdict.Reconstructable)
	}
	if len(verdict.Warnings) == 0 {
		t.Fatal("expected frame warning that CRC pass is insufficient")
	}
	if !contains(report.GlobalWarnings, "出现 CRC 通过但像素校验失败") {
		t.Fatalf("expected global CRC/pixel contradiction warning: %#v", report.GlobalWarnings)
	}
}

func TestKnownGoodPacketEnablesExactPacketRedecode(t *testing.T) {
	bundle := Bundle{
		Observations: []Observation{
			{Layer: LayerBitstream, Status: StatusFail, Frame: FrameRef{Key: "f0009", DecodeOrder: 9}, Check: "crc"},
			{Layer: LayerPixels, Status: StatusFail, TrustedReference: true, Frame: FrameRef{Key: "f0009", DecodeOrder: 9}, Check: "pixel_hash"},
		},
		Recovery: []Recovery{{FrameKey: "f0009", GoodPacket: true, BitExactDecoder: true}},
	}

	report, err := Analyze(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.FrameVerdicts[0].Reconstructable; got != ReconstructYes {
		t.Fatalf("reconstructable = %q, want yes", got)
	}
	if got := report.FrameVerdicts[0].Mode; got != "redecode_known_good_packet_with_bit_exact_decoder" {
		t.Fatalf("mode = %q", got)
	}
}

func TestEarliestDistinguishesDecodeOrderAndDisplayOrder(t *testing.T) {
	earlyDisplay := 0
	laterDisplay := 1
	bundle := Bundle{Observations: []Observation{
		{
			Layer: LayerBitstream, Status: StatusFail, Check: "crc",
			Frame: FrameRef{Key: "decoded-first-display-later", DecodeOrder: 1, DisplayOrder: &laterDisplay},
		},
		{
			Layer: LayerContainer, Status: StatusFail, Check: "pts",
			Frame: FrameRef{Key: "decoded-second-display-first", DecodeOrder: 2, DisplayOrder: &earlyDisplay},
		},
	}}

	report, err := Analyze(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if report.Earliest.FrameKey != "decoded-first-display-later" {
		t.Fatalf("decode earliest = %q", report.Earliest.FrameKey)
	}
	if report.EarliestDisplay == nil || report.EarliestDisplay.FrameKey != "decoded-second-display-first" {
		t.Fatalf("display earliest = %#v", report.EarliestDisplay)
	}
}

func trustedPasses(key string, order int) []Observation {
	frame := FrameRef{Key: key, DecodeOrder: order}
	return []Observation{
		{Layer: LayerBitstream, Status: StatusPass, Check: "bitstream_crc", Frame: frame},
		{Layer: LayerDecode, Status: StatusPass, Check: "decoder", Frame: frame},
		{Layer: LayerPixels, Status: StatusPass, TrustedReference: true, Check: "pixel_hash", Frame: frame},
	}
}

func verdictsByKey(verdicts []Verdict) map[string]Verdict {
	result := make(map[string]Verdict, len(verdicts))
	for _, verdict := range verdicts {
		result[verdict.FrameKey] = verdict
	}
	return result
}

func contains(values []string, prefix string) bool {
	for _, value := range values {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
