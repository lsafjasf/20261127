package analyze

import (
	"clipforensic/internal/demo"
	"testing"
)

func TestMusicLimiterProducesCandidateWindows(t *testing.T) {
	a := demo.Music()
	rep := Analyze(a, DefaultOptions(a.SampleRate))
	if len(rep.Candidates) < 2 {
		t.Fatalf("expected at least two clip candidates, got %d", len(rep.Candidates))
	}
	want := [][2]float64{{0.46, 0.80}, {1.32, 1.76}}
	for wi, window := range want {
		var found *Candidate
		for i := range rep.Candidates {
			c := &rep.Candidates[i]
			if c.StartSec < window[1] && c.EndSec > window[0] {
				found = c
				break
			}
		}
		if found == nil {
			t.Fatalf("window %v has no candidate: %+v", window, rep.Candidates)
		}
		if found.StartSec > window[0]+0.10 || found.EndSec < window[1]-0.08 {
			t.Fatalf("candidate %s %.3f-%.3f does not cover injected window %.3f-%.3f",
				found.ID, found.StartSec, found.EndSec, window[0], window[1])
		}
		if found.Confidence < 0.60 {
			t.Fatalf("candidate confidence too low: %.3f", found.Confidence)
		}
		if found.Plateau.LongestRun < 8 || found.Plateau.ExactFullScaleSamples == 0 {
			t.Fatalf("plateau evidence too weak: %+v", found.Plateau)
		}
		if found.Edge.SteepEdgeRatio < 0.05 {
			t.Fatalf("edge evidence too weak: %+v", found.Edge)
		}
		_ = wi
	}
}

func TestDistortedGuitarIsGuardedAsArtistic(t *testing.T) {
	a := demo.Guitar()
	rep := Analyze(a, DefaultOptions(a.SampleRate))
	if len(rep.Candidates) != 0 {
		t.Fatalf("artistic distortion must not become incident candidates, got %+v", rep.Candidates)
	}
	artistic := 0
	for _, c := range rep.Artistic {
		if c.Status == "artistic_distortion" {
			artistic++
			if c.Spectrum.EstimatedF0Hz < 120 || c.Spectrum.HarmonicStrength < 0.5 {
				t.Fatalf("guitar region lacks tonal evidence: f0=%.1f harmonic=%.2f",
					c.Spectrum.EstimatedF0Hz, c.Spectrum.HarmonicStrength)
			}
		}
	}
	if artistic < 5 {
		t.Fatalf("expected at least five guarded guitar notes, got %d", artistic)
	}
}

func TestCleanToneHasNoIncident(t *testing.T) {
	a := demo.Music()
	for c := range a.Samples {
		for i := range a.Samples[c] {
			a.Samples[c][i] *= 0.25
		}
	}
	rep := Analyze(a, DefaultOptions(a.SampleRate))
	if len(rep.Candidates) != 0 {
		t.Fatalf("clean quiet tone generated incidents: %d", len(rep.Candidates))
	}
}
