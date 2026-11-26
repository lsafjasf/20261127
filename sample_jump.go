package hdrframe

// SequenceJumpSample keeps frame/PTS identity correct but makes the metadata
// sequence number jump from 301 to 305. The player applies the frame-accurate
// record; the jump is reported but is not treated as a missing video frame.
func SequenceJumpSample() Input {
	scene := Int64Ptr(42)
	expected := []Metadata{
		sampleMeta("JMP-300", 300, scene, 40, 300, 1200, 1.5, -2),
		sampleMeta("JMP-301", 301, scene, 41, 301, 1300, 1.6, -3),
		sampleMeta("JMP-302-frame-right", 302, scene, 42, 302, 2600, 2.5, -12),
		sampleMeta("JMP-303", 303, scene, 43, 303, 2200, 2.1, -9),
		sampleMeta("JMP-304", 304, scene, 44, 304, 1800, 1.9, -6),
	}
	delivered := []Metadata{
		expected[0], expected[1],
		sampleMetaWithOrder("SEI-JUMP-305", "JMP-302-frame-right", 302, scene, 42, 305, 1, 2600, 2.5, -12),
		sampleMetaWithOrder("SEI-JUMP-306", "JMP-303", 303, scene, 43, 306, 1, 2200, 2.1, -9),
		sampleMetaWithOrder("SEI-JUMP-307", "JMP-304", 304, scene, 44, 307, 1, 1800, 1.9, -6),
	}
	for i := range delivered {
		delivered[i].Source = "transcoded_mp4_sample"
	}

	return Input{
		SchemaVersion: 1,
		Name:          "sequence_number_jump",
		FPS:           "25",
		Policy:        Policy{SequenceGap: "accept", MissingMetadata: "hold_last"},
		Targets: []Target{
			{Frame: 300, ExpectedSceneID: scene, Label: "stage-spotlight"},
			{Frame: 301, ExpectedSceneID: scene},
			{Frame: 302, ExpectedSceneID: scene, Label: "sequence-jump-but-pts-continuous"},
			{Frame: 303, ExpectedSceneID: scene},
			{Frame: 304, ExpectedSceneID: scene},
		},
		ExpectedMetadata: withFPS(expected, 25),
		Metadata:         withFPS(delivered, 25),
		PlayerEvents: []PlayerEvent{
			{
				TargetFrame: 302, AppliedVersion: "JMP-302-frame-right", AppliedSequence: Int64Ptr(305),
				Source:   "player_hdr10plus_debug_log",
				Evidence: "presented PTS=12.080; active T.35 sequence_number=305 with correct picture PTS",
			},
		},
	}
}

// Samples returns the three requested anomaly fixtures in a stable order.
func Samples() []Input {
	return []Input{DuplicateSample(), MissingFrameSample(), SequenceJumpSample()}
}

// Fixtures is an alias for Samples.
func Fixtures() []Input { return Samples() }
