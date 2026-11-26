package analyze

import (
	"math"
	"sort"

	"colorfix/internal/color"
	"colorfix/internal/video"
)

type BarSample struct {
	Index  int       `json:"index"`
	RGB601 color.RGB `json:"rgb_bt601"`
	RGB709 color.RGB `json:"rgb_bt709"`
}

type CandidateScore struct {
	Matrix    color.Matrix `json:"matrix"`
	Template  string       `json:"template"`
	Residual  float64      `json:"residual"`
	Coverage  float64      `json:"coverage"`
	Structure float64      `json:"structure"`
}

type BarAnalysis struct {
	Found      bool             `json:"found"`
	FrameTime  float64          `json:"frame_time"`
	Y          int              `json:"y"`
	XStart     int              `json:"x_start"`
	XEnd       int              `json:"x_end"`
	Candidates []CandidateScore `json:"candidates"`
	Samples    []BarSample      `json:"samples"`
}

func (b BarAnalysis) Score(matrix color.Matrix) CandidateScore {
	for _, c := range b.Candidates {
		if c.Matrix == matrix {
			return c
		}
	}
	return CandidateScore{Matrix: matrix}
}

type templateScore struct {
	tpl       color.BarsTemplate
	mse       float64
	coverage  float64
	structure float64
}

func AnalyzeBars(frame *video.Frame) BarAnalysis {
	best := BarAnalysis{FrameTime: frame.Time}
	yStep := maxInt(2, frame.Height/120)
	yEnd := frame.Height * 3 / 4
	bestMSE := math.Inf(1)
	const presenceResidual = 0.10
	presenceMSE := presenceResidual * presenceResidual
	for y := yStep; y < yEnd; y += yStep {
		samples := sampleBars(frame, y)
		score601 := scoreTemplates(samples, color.BT601)
		score709 := scoreTemplates(samples, color.BT709)
		frameBest := score601
		if score709.mse < score601.mse {
			frameBest = score709
		}
		if frameBest.mse < bestMSE {
			bestMSE = frameBest.mse
			best = BarAnalysis{
				Found:      frameBest.mse <= presenceMSE,
				FrameTime:  frame.Time,
				Y:          y,
				XStart:     0,
				XEnd:       frame.Width,
				Candidates: candidatesFromScores(score601, score709),
				Samples:    samples,
			}
		}
	}
	return best
}

func candidatesFromScores(a, b templateScore) []CandidateScore {
	return []CandidateScore{
		{
			Matrix: color.BT601, Template: a.tpl.Name,
			Residual: math.Sqrt(a.mse), Coverage: a.coverage, Structure: a.structure,
		},
		{
			Matrix: color.BT709, Template: b.tpl.Name,
			Residual: math.Sqrt(b.mse), Coverage: b.coverage, Structure: b.structure,
		},
	}
}

func sampleBars(frame *video.Frame, y int) []BarSample {
	n := 7
	barWidth := float64(frame.Width) / float64(n)
	half := maxInt(2, minInt(7, int(barWidth/8)))
	samples := make([]BarSample, 0, n)
	for i := 0; i < n; i++ {
		cx := int((float64(i) + 0.5) * barWidth)
		yn, un, vn := medianPatch(frame, cx, y, half)
		r601, g601, b601 := color.Decode(color.BT601, yn, un, vn)
		r709, g709, b709 := color.Decode(color.BT709, yn, un, vn)
		samples = append(samples, BarSample{
			Index:  i,
			RGB601: color.RGB{R: r601, G: g601, B: b601},
			RGB709: color.RGB{R: r709, G: g709, B: b709},
		})
	}
	return samples
}

func scoreTemplates(samples []BarSample, matrix color.Matrix) templateScore {
	best := templateScore{mse: math.Inf(1)}
	for _, tpl := range color.Templates {
		var sum float64
		var covered int
		var structured int
		for i, sample := range samples {
			actual := sample.RGB601
			if matrix == color.BT709 {
				actual = sample.RGB709
			}
			expected := tpl.Colors[i]
			residual := color.ResidualRGB(actual, expected)
			sum += residual
			if math.Sqrt(residual) <= 0.11 {
				covered++
			}
			nearest := i
			nearestDist := math.Inf(1)
			for j, candidate := range tpl.Colors {
				d := color.ResidualRGB(actual, candidate)
				if d < nearestDist {
					nearestDist = d
					nearest = j
				}
			}
			if nearest == i {
				structured++
			}
		}
		score := templateScore{
			tpl:       tpl,
			mse:       sum / float64(len(samples)),
			coverage:  float64(covered) / float64(len(samples)),
			structure: float64(structured) / float64(len(samples)),
		}
		if score.mse < best.mse {
			best = score
		}
	}
	return best
}

func medianPatch(frame *video.Frame, cx, cy, half int) (y, u, v float64) {
	x0 := maxInt(0, cx-half)
	x1 := minInt(frame.Width-1, cx+half)
	y0 := maxInt(0, cy-half)
	y1 := minInt(frame.Height-1, cy+half)
	n := (x1 - x0 + 1) * (y1 - y0 + 1)
	ys := make([]float64, 0, n)
	us := make([]float64, 0, n)
	vs := make([]float64, 0, n)
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			yn, un, vn := frame.YUV(px, py)
			ys = append(ys, yn)
			us = append(us, un)
			vs = append(vs, vn)
		}
	}
	sort.Float64s(ys)
	sort.Float64s(us)
	sort.Float64s(vs)
	return ys[len(ys)/2], us[len(us)/2], vs[len(vs)/2]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
