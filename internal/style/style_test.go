package style

import (
	"testing"
)

func TestClassifierDistinguishesLiftFromNight(t *testing.T) {
	p := DefaultParams()

	// Clean, uniform, elevated floor with virtually no zero pixels.
	liftY := make([]float64, 10000)
	for i := range liftY {
		liftY[i] = 22.0/255.0 + float64(i%3)/255.0
	}
	liftFeat := ExtractFeatures(liftY, 1.5/255.0, 1.0/255.0)
	isLift, liftScore := Classify(liftFeat, p, 0.62)
	if !isLift {
		t.Fatalf("clean lift not detected, score %.3f", liftScore)
	}

	// Noisy true night: near-zero floor, heavy noise.
	nightY := make([]float64, 10000)
	for i := range nightY {
		v := float64(i%5) / 255.0
		if i%20 == 0 {
			v = 0
		}
		nightY[i] = v
	}
	nightFeat := ExtractFeatures(nightY, 8.0/255.0, 3.0/255.0)
	isNight, nightScore := Classify(nightFeat, p, 0.62)
	if isNight {
		t.Fatalf("true night misclassified as lift, score %.3f", nightScore)
	}
	if liftScore <= nightScore {
		t.Fatalf("lift score %.3f should exceed night score %.3f", liftScore, nightScore)
	}
}

func TestTemporalVoteRemovesSingleFrame(t *testing.T) {
	scores := []float64{0.2, 0.2, 0.9, 0.2, 0.2}
	v := TemporalVote(scores, 0.62, 3)
	for i, want := range []bool{false, false, false, false, false} {
		if v[i] != want {
			t.Fatalf("frame %d vote %v want %v", i, v[i], want)
		}
	}
	consistent := []float64{0.9, 0.9, 0.9, 0.2, 0.2}
	v2 := TemporalVote(consistent, 0.62, 3)
	if !v2[1] {
		t.Fatal("consistent lift window should vote true")
	}
}
