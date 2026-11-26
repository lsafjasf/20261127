package analyze

import (
	"testing"

	"colorfix/internal/color"
	"colorfix/internal/fixture"
	"colorfix/internal/video"
)

func TestBarsFavorBT601(t *testing.T) {
	raw := fixture.YUV444Frame(720, 576)
	frame, err := video.NewFrame(720, 576, raw, video.RangeLimited)
	if err != nil {
		t.Fatal(err)
	}
	bars := AnalyzeBars(frame)
	if !bars.Found {
		t.Fatal("synthetic 75% bars were not detected")
	}
	s601 := bars.Score(color.BT601)
	s709 := bars.Score(color.BT709)
	if s601.Template != "75%" {
		t.Errorf("template = %q, want 75%%", s601.Template)
	}
	if s601.Residual > 0.01 {
		t.Errorf("BT.601 residual %.4f is unexpectedly high", s601.Residual)
	}
	if s709.Residual < 0.03 {
		t.Errorf("BT.709 residual %.4f is unexpectedly low", s709.Residual)
	}
	if s601.Residual >= s709.Residual {
		t.Errorf("residuals do not favor BT.601: 601 %.4f, 709 %.4f", s601.Residual, s709.Residual)
	}
}

func TestDecisionRelabelsMislabeled709(t *testing.T) {
	bars := syntheticBars(0.002, 0.053)
	decision := Decide(bars, color.BT709, "bt709", DefaultDecisionOptions())
	if decision.Status != StatusRelabeled || decision.Selected != color.BT601 {
		t.Fatalf("decision = %+v, want relabeled bt601", decision)
	}
}

func TestDecisionRetainsCandidatesWhenClose(t *testing.T) {
	bars := syntheticBars(0.030, 0.035)
	decision := Decide(bars, color.BT709, "bt709", DefaultDecisionOptions())
	if decision.Status != StatusAmbiguous {
		t.Fatalf("status = %q, want ambiguous", decision.Status)
	}
	if decision.Unique || decision.Selected != "" {
		t.Fatalf("ambiguous decision was forced: %+v", decision)
	}
	if len(decision.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(decision.Candidates))
	}
}

func TestNoBarsIsInsufficient(t *testing.T) {
	raw := make([]byte, 720*576*3)
	for i := range raw {
		raw[i] = 128
	}
	frame, err := video.NewFrame(720, 576, raw, video.RangeLimited)
	if err != nil {
		t.Fatal(err)
	}
	bars := AnalyzeBars(frame)
	decision := Decide(bars, color.BT709, "bt709", DefaultDecisionOptions())
	if decision.Status != StatusInsufficient || decision.Selected != "" {
		t.Fatalf("decision = %+v, want insufficient without selection", decision)
	}
}

func syntheticBars(r601, r709 float64) BarAnalysis {
	return BarAnalysis{
		Found: true,
		Candidates: []CandidateScore{
			{Matrix: color.BT601, Template: "75%", Residual: r601, Coverage: 0.95, Structure: 0.95},
			{Matrix: color.BT709, Template: "75%", Residual: r709, Coverage: 0.95, Structure: 0.95},
		},
	}
}
