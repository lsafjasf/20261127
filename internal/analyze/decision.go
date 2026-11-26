package analyze

import (
	"fmt"
	"math"

	"colorfix/internal/color"
)

const (
	StatusVerified     = "verified"
	StatusRelabeled    = "relabeled"
	StatusLabeled      = "labeled"
	StatusAmbiguous    = "ambiguous"
	StatusInsufficient = "insufficient_evidence"
)

type Evidence struct {
	Source      string `json:"source"`
	Observation string `json:"observation"`
	Effect      string `json:"effect"`
}

type Candidate struct {
	Matrix    color.Matrix `json:"matrix"`
	Label     string       `json:"label"`
	Residual  float64      `json:"bars_residual"`
	Preferred bool         `json:"preferred"`
}

type Decision struct {
	Status     string       `json:"status"`
	Selected   color.Matrix `json:"selected_matrix,omitempty"`
	Candidates []Candidate  `json:"candidates"`
	Unique     bool         `json:"unique"`
	Reason     string       `json:"reason"`
	Evidence   []Evidence   `json:"evidence"`
}

type DecisionOptions struct {
	MaxBarResidual   float64
	MinCoverage      float64
	MinStructure     float64
	MinMargin        float64
	MinResidualRatio float64
}

func DefaultDecisionOptions() DecisionOptions {
	return DecisionOptions{
		MaxBarResidual:   0.075,
		MinCoverage:      0.70,
		MinStructure:     0.70,
		MinMargin:        0.015,
		MinResidualRatio: 1.60,
	}
}

func Decide(bars BarAnalysis, declared color.Matrix, declaredName string, opts DecisionOptions) Decision {
	score601 := bars.Score(color.BT601)
	score709 := bars.Score(color.BT709)
	evidence := evidenceFor(bars, score601, score709, declared, declaredName)
	if !bars.Found {
		return Decision{
			Status: StatusInsufficient, Unique: false,
			Candidates: candidatesBoth(bars, declared, false), Evidence: evidence,
			Reason: "no color-bar frame with a reliable seven-bar fit was found",
		}
	}
	winner := color.BT601
	best, other := score601, score709
	if score709.Residual < score601.Residual {
		winner, best, other = color.BT709, score709, score601
	}
	margin := other.Residual - best.Residual
	ratio := math.Inf(1)
	if best.Residual > 0 {
		ratio = other.Residual / best.Residual
	}
	unique := best.Residual <= opts.MaxBarResidual &&
		best.Coverage >= opts.MinCoverage &&
		best.Structure >= opts.MinStructure &&
		margin >= opts.MinMargin &&
		ratio >= opts.MinResidualRatio

	if !unique {
		return Decision{
			Status: StatusAmbiguous, Unique: false,
			Candidates: candidatesBoth(bars, declared, true), Evidence: evidence,
			Reason: fmt.Sprintf(
				"color bars fit both matrices too closely (residuals %.4f vs %.4f, margin %.4f)",
				score601.Residual, score709.Residual, margin),
		}
	}

	decision := Decision{Selected: winner, Unique: true, Evidence: evidence}
	switch {
	case declared == winner:
		decision.Status = StatusVerified
		decision.Reason = "color-bar residual and container declaration agree"
	case declared != "":
		decision.Status = StatusRelabeled
		decision.Reason = fmt.Sprintf("color-bar residual strongly favors %s over declared %s", winner, declaredName)
	default:
		decision.Status = StatusLabeled
		decision.Reason = "color-bar residual is decisive; the input had no usable matrix declaration"
	}
	decision.Candidates = []Candidate{candidate(winner, bars, true)}
	return decision
}

func candidatesBoth(bars BarAnalysis, declared color.Matrix, barsPreferred bool) []Candidate {
	ordered := []color.Matrix{color.BT601, color.BT709}
	if declared == color.BT709 {
		ordered = []color.Matrix{color.BT709, color.BT601}
	}
	out := make([]Candidate, 0, 2)
	for _, m := range ordered {
		preferred := barsPreferred && bars.Score(m).Residual <= bars.Score(otherMatrix(m)).Residual
		out = append(out, candidate(m, bars, preferred))
	}
	return out
}

func candidate(m color.Matrix, bars BarAnalysis, preferred bool) Candidate {
	return Candidate{
		Matrix: m, Label: m.Label(), Residual: bars.Score(m).Residual,
		Preferred: preferred,
	}
}

func otherMatrix(m color.Matrix) color.Matrix {
	if m == color.BT601 {
		return color.BT709
	}
	return color.BT601
}

func evidenceFor(bars BarAnalysis, s601, s709 CandidateScore, declared color.Matrix, declaredName string) []Evidence {
	ev := []Evidence{{
		Source: "color_bars",
		Observation: fmt.Sprintf(
			"found=%t at %.3fs, y=%d, templates=%s/%s, coverage=%.0f%%/%.0f%%",
			bars.Found, bars.FrameTime, bars.Y, s601.Template, s709.Template,
			s601.Coverage*100, s709.Coverage*100),
		Effect: "establishes that a structured reference pattern is present; it does not by itself choose a matrix",
	}, {
		Source: "matrix_residual",
		Observation: fmt.Sprintf(
			"BT.601=%.4f, BT.709=%.4f, delta=%.4f",
			s601.Residual, s709.Residual, s709.Residual-s601.Residual),
		Effect: "compares each candidate decode against the same measured bar patches",
	}}
	if declared != "" {
		ev = append(ev, Evidence{
			Source:      "container_declaration",
			Observation: fmt.Sprintf("%s (%s)", declaredName, declared),
			Effect:      "is treated as prior metadata, not as ground truth; a strong residual conflict can overturn it",
		})
	} else {
		ev = append(ev, Evidence{
			Source: "container_declaration", Observation: "none",
			Effect: "leaves selection entirely to measured evidence",
		})
	}
	return ev
}
