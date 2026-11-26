package hdrframe

// DuplicateSample builds two conflicting records with sequence/frame 122.
// The bitstream order makes B the timeline-selected version, while the player
// evidence intentionally shows that A was applied.
func DuplicateSample() Input {
	scene := Int64Ptr(42)
	expected := []Metadata{
		sampleMeta("EXP-120", 120, scene, 0, 120, 1000, 1.4, 2),
		sampleMeta("EXP-121", 121, scene, 1, 121, 1120, 1.5, 0),
		sampleMeta("HM-122-B-right", 122, scene, 2, 122, 2200, 2.4, -8),
		sampleMeta("EXP-123", 123, scene, 3, 123, 1800, 2.0, -5),
		sampleMeta("EXP-124", 124, scene, 4, 124, 1600, 1.8, -4),
	}
	delivered := cloneMetadata(expected[:2])
	delivered = append(delivered,
		sampleMetaWithOrder("SEI-122-A-old", "HM-122-A-old", 122, scene, 2, 122, 1, 800, 1.1, 9),
		sampleMetaWithOrder("SEI-122-B-right", "HM-122-B-right", 122, scene, 2, 122, 2, 2200, 2.4, -8),
		expected[3], expected[4],
	)

	return Input{
		SchemaVersion: 1,
		Name:          "duplicate_conflicting_metadata",
		FPS:           "24",
		Policy:        Policy{Duplicate: "latest", MissingMetadata: "hold_last"},
		Targets: []Target{
			{Frame: 120, ExpectedSceneID: scene, Label: "neon-sign"},
			{Frame: 121, ExpectedSceneID: scene},
			{Frame: 122, ExpectedSceneID: scene, Label: "duplicate-sei"},
			{Frame: 123, ExpectedSceneID: scene},
			{Frame: 124, ExpectedSceneID: scene},
		},
		ExpectedMetadata: expected,
		Metadata:         delivered,
		PlayerEvents: []PlayerEvent{
			{
				TargetFrame: 122, AppliedVersion: "HM-122-A-old",
				Source:   "player_hdr10plus_debug_log",
				Evidence: "T.35 active metadata digest=old-a; tone mapper used 800 nits/+9 hue",
			},
		},
	}
}

func sampleMeta(version string, frame int64, scene *int64, sceneFrame int64, sequence int64, peak, gain, hue float64) Metadata {
	return sampleMetaWithOrder(version+"-id", version, frame, scene, sceneFrame, sequence, 0, peak, gain, hue)
}

func sampleMetaWithOrder(id, version string, frame int64, scene *int64, sceneFrame, sequence int64, order int, peak, gain, hue float64) Metadata {
	frameCopy := frame
	return Metadata{
		ID: id, Version: version, Sequence: sequence, Frame: &frameCopy,
		SceneID: scene, SceneFrameIndex: Int64Ptr(sceneFrame),
		EffectiveSeconds: Float64Ptr(float64(frame) / 24),
		IngestOrder:      order, BitstreamOrder: order, Source: "hdr10plus_sei_json",
		Highlight: HighlightParams{TargetPeakNits: peak, PeakNits: peak, Gain: gain, KneeX: 0.72, KneeY: 0.88},
		Color:     ColorParams{HueShiftDegrees: hue, Saturation: 1.12},
	}
}
