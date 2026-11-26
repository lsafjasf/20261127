package flatfield

import (
	"math"
)

func fitComponents(samples []cellSample, z []float64, cfg Options) ([6]float64, []float64, string, error) {
	n := len(samples)
	lensLogs := make([]float64, n)
	var beta [6]float64

	if cfg.Lens != nil {
		if err := cfg.Lens.Validate(); err != nil {
			return beta, nil, "", err
		}
		y := make([]float64, n)
		x := make([][]float64, n)
		w := make([]float64, n)
		for i, s := range samples {
			lr, err := cfg.Lens.ComponentResponseAt(KindLens, s.u, s.v)
			if err != nil {
				return beta, nil, "", err
			}
			lensLogs[i] = math.Log(lr)
			y[i] = z[i] - lensLogs[i]
			x[i] = []float64{1, s.dx, s.dy}
			w[i] = s.weight
		}
		b := solveWeightedLeastSquares(x, y, w)
		if b == nil {
			return beta, nil, "", errFitFailed
		}
		copy(beta[:3], b)
	} else {
		minDim := minInt(cfg.GridWidth, cfg.GridHeight)
		radialTerms := 0
		switch {
		case minDim >= 9:
			radialTerms = 3
		case minDim >= 6:
			radialTerms = 2
		case minDim >= 3:
			radialTerms = 1
		}
		p := 3 + radialTerms
		x := make([][]float64, n)
		y := make([]float64, n)
		w := make([]float64, n)
		for i, s := range samples {
			row := make([]float64, p)
			row[0], row[1], row[2] = 1, s.dx, s.dy
			if radialTerms >= 1 {
				row[3] = s.r2
			}
			if radialTerms >= 2 {
				row[4] = s.r2 * s.r2
			}
			if radialTerms >= 3 {
				row[5] = s.r2 * s.r2 * s.r2
			}
			x[i], y[i], w[i] = row, z[i], s.weight
		}
		b := solveWeightedLeastSquares(x, y, w)
		if b == nil && radialTerms > 0 {
			fallback := make([][]float64, n)
			for i, s := range samples {
				fallback[i] = []float64{1, s.dx, s.dy}
			}
			b = solveWeightedLeastSquares(fallback, y, w)
		}
		if b == nil {
			return beta, nil, "", errFitFailed
		}
		copy(beta[:p], b)
		for i, s := range samples {
			lensLogs[i] = beta[3]*s.r2 + beta[4]*s.r2*s.r2 + beta[5]*s.r2*s.r2*s.r2
		}
	}

	gradMin, gradMax := math.Inf(1), math.Inf(-1)
	for _, s := range samples {
		g := beta[1]*s.dx + beta[2]*s.dy
		gradMin = math.Min(gradMin, g)
		gradMax = math.Max(gradMax, g)
	}
	if gradMax-gradMin < cfg.MinGradientLog {
		beta[1], beta[2] = 0, 0
	}
	return beta, lensLogs, lensReferenceName(cfg.Lens), nil
}

func lensReferenceName(t *Table) string {
	if t == nil {
		return ""
	}
	if t.Schema != "" {
		return "supplied_table:" + t.Schema
	}
	return "supplied_table"
}

func buildResponses(tab *Table, samples []cellSample, z []float64, refLog float64, beta [6]float64, lensLogs []float64) {
	for i, s := range samples {
		gradLog := beta[1]*s.dx + beta[2]*s.dy
		panelResidualLog := z[i] - refLog - gradLog - lensLogs[i]
		panelFullLog := z[i] - refLog - lensLogs[i]
		tab.ObservedResponse[i] = safeExp(z[i] - refLog)
		tab.GradientResponse[i] = safeExp(gradLog)
		tab.LensResponse[i] = safeExp(lensLogs[i])
		tab.PanelResidualResponse[i] = safeExp(panelResidualLog)
		tab.PanelResponse[i] = safeExp(panelFullLog)
	}
}

func safeExp(v float64) float64 {
	const limit = 27.631021115928547 // log(1e12)
	v = clampFloat(v, -limit, limit)
	r := math.Exp(v)
	if r <= 0 || math.IsNaN(r) {
		return 1e-12
	}
	return r
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
