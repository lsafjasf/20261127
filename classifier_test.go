package tearcheck

import "testing"

func TestClassifyHardwareSync(t *testing.T) {
	o := Observation{
		FrameIndex:          12,
		Time:                0.4,
		SeamY:               37,
		IntraFrameTear:      0.88,
		SeamCoverage:        0.91,
		AdjacentFrameMatch:  0.94,
		MatchContrast:       0.86,
		RepeatedSide:        SideBelow,
		TimingFault:         0.78,
		ArbitraryScanline:   0.82,
		BlockAlignment:      0.12,
		BlockArtifact:       0.18,
		TemporalBlockRepeat: 0.20,
	}
	finding := ClassifyObservation(o)
	if finding.Cause != CauseHardwareSync {
		t.Fatalf("cause = %q, want hardware_sync; scores=%#v manual=%+v", finding.Cause, finding.Scores, finding.Manual)
	}
	if finding.Confidence != ConfidenceHigh || finding.ReviewRequired {
		t.Fatalf("confidence=%q review=%v, want high/no review", finding.Confidence, finding.ReviewRequired)
	}
}

func TestClassifyEncodingErrorWithDecoderEvidence(t *testing.T) {
	o := Observation{
		FrameIndex:          8,
		SeamY:               48,
		IntraFrameTear:      0.76,
		SeamCoverage:        0.42,
		AdjacentFrameMatch:  0.82,
		MatchContrast:       0.70,
		BlockAlignment:      1,
		BlockArtifact:       0.86,
		TemporalBlockRepeat: 0.78,
		CodecError:          1,
	}
	finding := ClassifyObservation(o)
	if finding.Cause != CauseEncoding {
		t.Fatalf("cause=%q want encoding_error scores=%#v manual=%+v", finding.Cause, finding.Scores, finding.Manual)
	}
	if finding.ReviewRequired || finding.Confidence == ConfidenceLow {
		t.Fatalf("encoding finding should be actionable: %+v", finding)
	}
}

func TestClassifyContentEffect(t *testing.T) {
	o := Observation{
		FrameIndex:         21,
		SeamY:              55,
		IntraFrameTear:     0.83,
		SeamCoverage:       0.72,
		AdjacentFrameMatch: 0.18,
		MatchContrast:      0.20,
		ContentMarker:      1,
		ChromaEffect:       0.91,
		EffectCoherence:    0.82,
		BlockAlignment:     0.2,
	}
	finding := ClassifyObservation(o)
	if finding.Cause != CauseContent {
		t.Fatalf("cause=%q want content_effect scores=%#v manual=%+v", finding.Cause, finding.Scores, finding.Manual)
	}
	if finding.Confidence != ConfidenceHigh {
		t.Fatalf("confidence=%q want high", finding.Confidence)
	}
}

func TestAmbiguousEvidenceStaysManual(t *testing.T) {
	o := Observation{
		FrameIndex:         3,
		SeamY:              48,
		IntraFrameTear:     0.72,
		SeamCoverage:       0.66,
		AdjacentFrameMatch: 0.58,
		MatchContrast:      0.40,
		BlockAlignment:     1,
		BlockArtifact:      0.52,
		CodecError:         0.25,
	}
	finding := ClassifyObservation(o)
	if finding.Cause != CauseManual || !finding.ReviewRequired {
		t.Fatalf("cause=%q review=%v want manual_review", finding.Cause, finding.ReviewRequired)
	}
	if finding.Manual == nil || len(finding.Manual.PossibleCauses) < 2 {
		t.Fatalf("manual item must retain possible causes: %+v", finding.Manual)
	}
	if finding.Manual.Observation.SeamY != 48 {
		t.Fatalf("manual item did not retain observation: %+v", finding.Manual)
	}
}
