package hdrframe

import "testing"

func TestAdaptToolJSONSceneIntervals(t *testing.T) {
	data := []byte(`{
      "SceneInfo": [
        {"SceneId": 42, "FirstFrameIndex": 10}
      ],
      "Metadata": [
        {"SequenceFrameIndex": 10, "SceneFrameIndex": 0, "TargetedSystemDisplayMaximumLuminance": 1000},
        {"SequenceFrameIndex": 11, "SceneFrameIndex": 1, "TargetedSystemDisplayMaximumLuminance": 1800}
      ]
    }`)
	input, err := AdaptToolJSON(data, "24")
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Metadata) != 2 || len(input.Targets) != 2 {
		t.Fatalf("metadata=%d targets=%d", len(input.Metadata), len(input.Targets))
	}
	if input.Metadata[1].SceneID == nil || *input.Metadata[1].SceneID != 42 {
		t.Fatalf("scene id=%+v, want 42 from SceneInfo", input.Metadata[1].SceneID)
	}
	if input.Metadata[1].Highlight.TargetPeakNits != 1800 {
		t.Fatalf("highlight=%+v", input.Metadata[1].Highlight)
	}
	report, err := Audit(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.TargetFrames != 2 {
		t.Fatalf("target frames=%d", report.Summary.TargetFrames)
	}
}
