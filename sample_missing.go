package hdrframe

// MissingFrameSample removes the transcoded record for frame 202 while source
// metadata still contains the corrected highlight/hue version. The player log
// shows that frame 202 stays on HM-201.
func MissingFrameSample() Input {
	scene := Int64Ptr(42)
	expected := []Metadata{
		sampleMeta("SRC-200", 200, scene, 10, 200, 900, 1.2, 3),
		sampleMeta("SRC-201", 201, scene, 11, 201, 980, 1.3, 2),
		sampleMeta("HM-202-required", 202, scene, 12, 202, 2400, 2.7, -10),
		sampleMeta("SRC-203", 203, scene, 13, 203, 2100, 2.2, -7),
		sampleMeta("SRC-204", 204, scene, 14, 204, 1700, 1.9, -5),
	}
	delivered := []Metadata{expected[0], expected[1], expected[3], expected[4]}
	for i := range delivered {
		delivered[i].Source = "transcoded_h265_sei"
	}

	return Input{
		SchemaVersion: 1,
		Name:          "missing_frame_metadata",
		FPS:           "24000/1001",
		Policy:        Policy{MissingMetadata: "hold_last", Duplicate: "latest"},
		Targets: []Target{
			{Frame: 200, ExpectedSceneID: scene, Label: "carousel-lights"},
			{Frame: 201, ExpectedSceneID: scene},
			{Frame: 202, ExpectedSceneID: scene, Label: "missing-dynamic-metadata"},
			{Frame: 203, ExpectedSceneID: scene},
			{Frame: 204, ExpectedSceneID: scene},
		},
		ExpectedMetadata: withFPS(expected, 24000.0/1001.0),
		Metadata:         withFPS(delivered, 24000.0/1001.0),
		PlayerEvents: []PlayerEvent{
			{
				TargetFrame: 202, AppliedVersion: "SRC-201",
				Source:   "player_hdr10plus_debug_log",
				Evidence: "no new T.35 metadata at this picture; active id remains sequence 201",
			},
		},
	}
}

func withFPS(records []Metadata, fps float64) []Metadata {
	out := cloneMetadata(records)
	for i := range out {
		if frame, ok := out[i].frameValue(); ok {
			out[i].EffectiveSeconds = Float64Ptr(float64(frame) / fps)
		}
	}
	return out
}
