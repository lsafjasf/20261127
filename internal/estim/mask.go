package estim

import (
	"math"
	"sort"
)

// Mask is a radial quadratic lens/black-level surface. rho is the distance
// from frame center normalized so frame corners map to rho=1:
// b(rho) = c0 + c1*rho + c2*rho^2. C0 is the center black level and the
// value at rho=1 is the corner black level.
type Mask struct {
	C0, C1, C2           float64
	Base, Corner, Spread float64
	RSquared             float64
}

// Rho converts normalized frame coordinates (nx,ny in [-1,1]) to the
// corner-normalized radius, clamped to the sampled domain [0,1].
func Rho(nx, ny float64) float64 {
	rho := math.Hypot(nx, ny) / math.Sqrt2
	if rho > 1 {
		return 1
	}
	return rho
}

func radialBasis(rho float64) [3]float64 {
	return [3]float64{1, rho, rho * rho}
}

// AtR evaluates the surface at a corner-normalized radius.
func (m *Mask) AtR(rho float64) float64 {
	b := radialBasis(rho)
	return m.C0*b[0] + m.C1*b[1] + m.C2*b[2]
}

// At evaluates the surface at normalized frame coordinates.
func (m *Mask) At(nx, ny float64) float64 {
	return m.AtR(Rho(nx, ny))
}

type radialBin struct {
	values []float64
	weight float64
}

type radialObs struct {
	rho, v, w float64
}

// FitMask estimates the radial black-level surface directly from pixels.
// Pixels are accumulated into radial bins; each bin's black level is the
// mean of its lowest 2%, which tracks the local floor without the tile-min
// bias of grid cells that straddle the radial gradient. A weighted least
// squares fit plus one Huber re-weighting pass produces the final surface.
func FitMask(y []float64, w, h, bins int) *Mask {
	if bins < 4 {
		bins = 16
	}
	rb := make([]radialBin, bins)
	for row := 0; row < h; row++ {
		ny := (float64(row)+0.5)/float64(h)*2 - 1
		base := row * w
		for col := 0; col < w; col++ {
			nx := (float64(col)+0.5)/float64(w)*2 - 1
			idx := int(Rho(nx, ny) * float64(bins-1))
			rb[idx].values = append(rb[idx].values, y[base+col])
		}
	}

	rows := make([]radialObs, 0, bins)
	for i, bin := range rb {
		if len(bin.values) < 16 {
			continue
		}
		sort.Float64s(bin.values)
		k := len(bin.values) / 50
		if k < 4 {
			k = 4
		}
		if k > len(bin.values)/4 {
			k = len(bin.values) / 4
		}
		sum, mean := 0.0, 0.0
		for j := 0; j < k; j++ {
			sum += bin.values[j]
		}
		floor := sum / float64(k)
		for _, v := range bin.values {
			mean += v
		}
		mean /= float64(len(bin.values))
		rho := (float64(i) + 0.5) / float64(bins-1)
		weight := 1.0 / (1 + math.Pow(mean/0.2, 4))
		rows = append(rows, radialObs{rho, floor, weight})
	}

	weights := make([]float64, len(rows))
	for i, r := range rows {
		weights[i] = r.w
	}
	coef := solveRadial(rows, weights)

	resid := make([]float64, len(rows))
	for i, r := range rows {
		resid[i] = r.v - evalRadial(coef, r.rho)
	}
	med := medianF(resid)
	abs := make([]float64, len(rows))
	for i := range abs {
		abs[i] = math.Abs(resid[i] - med)
	}
	mad := medianF(abs)
	k := 1.345 * (mad/0.6745 + 1e-6)
	for i, r := range rows {
		if a := math.Abs(resid[i]); a > k {
			weights[i] = r.w * k / a
		} else {
			weights[i] = r.w
		}
	}
	coef = solveRadial(rows, weights)

	m := &Mask{C0: coef[0], C1: coef[1], C2: coef[2]}
	if m.C0 < 0 {
		m.C0 = 0
	}
	m.Base = m.AtR(0)
	m.Corner = m.AtR(1)
	minV, maxV := math.Inf(1), math.Inf(-1)
	var sse, sst float64
	wm, sw := 0.0, 0.0
	for _, r := range rows {
		wm += r.w * r.v
		sw += r.w
	}
	wm /= sw
	for _, r := range rows {
		p := m.AtR(r.rho)
		minV = math.Min(minV, p)
		maxV = math.Max(maxV, p)
		sse += r.w * (r.v - p) * (r.v - p)
		sst += r.w * (r.v - wm) * (r.v - wm)
	}
	m.Spread = maxV - minV
	if sst > 0 {
		m.RSquared = 1 - sse/sst
		if m.RSquared < 0 {
			m.RSquared = 0
		}
	}
	return m
}

func evalRadial(c [3]float64, rho float64) float64 {
	b := radialBasis(rho)
	return c[0]*b[0] + c[1]*b[1] + c[2]*b[2]
}

func solveRadial(rows []radialObs, weights []float64) [3]float64 {
	var ata [3][3]float64
	var atb [3]float64
	for i, r := range rows {
		b := radialBasis(r.rho)
		w := weights[i]
		for p := 0; p < 3; p++ {
			atb[p] += w * b[p] * r.v
			for q := 0; q < 3; q++ {
				ata[p][q] += w * b[p] * b[q]
			}
		}
	}
	return gaussianSolve3(ata, atb)
}

func gaussianSolve3(a [3][3]float64, b [3]float64) [3]float64 {
	n := 3
	for col := 0; col < n; col++ {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(a[r][col]) > math.Abs(a[pivot][col]) {
				pivot = r
			}
		}
		a[pivot], a[col] = a[col], a[pivot]
		b[pivot], b[col] = b[col], b[pivot]
		if math.Abs(a[col][col]) < 1e-12 {
			a[col][col] = 1e-12
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := a[r][col] / a[col][col]
			for kk := col; kk < n; kk++ {
				a[r][kk] -= f * a[col][kk]
			}
			b[r] -= f * b[col]
		}
	}
	var x [3]float64
	for i := range x {
		x[i] = b[i] / a[i][i]
	}
	return x
}

func medianF(v []float64) float64 {
	cp := append([]float64(nil), v...)
	sort.Float64s(cp)
	if len(cp) == 0 {
		return 0
	}
	return cp[len(cp)/2]
}
