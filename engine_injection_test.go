package iccproof

import (
	"context"
	"testing"
)

type captureEngine struct {
	intents []Intent
}

func (e *captureEngine) ProofSample(_ context.Context, intent Intent, _ Sample, _ [3]float64) (EngineOutput, error) {
	e.intents = append(e.intents, intent)
	lab := Lab{L: 50, A: 1, B: -1}
	return EngineOutput{
		SourceLab: &lab, MediaLab: &lab, MediaColorSpace: "RGB",
		MediaValues: []float64{0.5, 0.5, 0.5}, MediaEncoded: []float64{0.5, 0.5, 0.5},
	}, nil
}
func (e *captureEngine) Close() error { return nil }

func TestProofPassesExplicitIntentsToEngine(t *testing.T) {
	source, media := testProfiles()
	engine := &captureEngine{}
	_, err := Proof(Request{
		Source: source, Media: media, Engine: engine,
		Intents: []Intent{IntentAbsoluteColorimetric, IntentPerceptual},
		Samples: []Sample{{RGB: []float64{0.1, 0.2, 0.3}}, {RGB: []float64{0.4, 0.5, 0.6}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Intent{
		IntentAbsoluteColorimetric, IntentAbsoluteColorimetric,
		IntentPerceptual, IntentPerceptual,
	}
	if len(engine.intents) != len(want) {
		t.Fatalf("intents=%v want %v", engine.intents, want)
	}
	for i := range want {
		if engine.intents[i] != want[i] {
			t.Fatalf("intent order %v want %v", engine.intents, want)
		}
	}
}
