package flatfield

import "math"

// solveWeightedLeastSquares solves argmin sum w_i (X_i*beta-y_i)^2.
// A tiny ridge term keeps low-resolution or rank-limited grids finite.
func solveWeightedLeastSquares(x [][]float64, y, weights []float64) []float64 {
	n := len(x)
	if n == 0 || len(y) != n || len(weights) != n {
		return nil
	}
	p := len(x[0])
	ata := make([][]float64, p)
	for i := range ata {
		ata[i] = make([]float64, p+1)
	}

	scale := 0.0
	for i := 0; i < n; i++ {
		w := weights[i]
		if w <= 0 {
			continue
		}
		for j := 0; j < p; j++ {
			xij := x[i][j]
			for k := j; k < p; k++ {
				ata[j][k] += w * xij * x[i][k]
			}
			ata[j][p] += w * xij * y[i]
		}
		scale += w
	}
	for j := 0; j < p; j++ {
		for k := j + 1; k < p; k++ {
			ata[k][j] = ata[j][k]
		}
		if j > 0 {
			ata[j][j] += 1e-9 * math.Max(1, scale)
		}
	}

	for col := 0; col < p; col++ {
		pivot := col
		best := math.Abs(ata[col][col])
		for row := col + 1; row < p; row++ {
			if v := math.Abs(ata[row][col]); v > best {
				best, pivot = v, row
			}
		}
		if best < 1e-18 {
			return nil
		}
		if pivot != col {
			ata[col], ata[pivot] = ata[pivot], ata[col]
		}
		pv := ata[col][col]
		for j := col; j <= p; j++ {
			ata[col][j] /= pv
		}
		for row := 0; row < p; row++ {
			if row == col {
				continue
			}
			factor := ata[row][col]
			if factor == 0 {
				continue
			}
			for j := col; j <= p; j++ {
				ata[row][j] -= factor * ata[col][j]
			}
		}
	}
	beta := make([]float64, p)
	for i := range beta {
		beta[i] = ata[i][p]
		if math.IsNaN(beta[i]) || math.IsInf(beta[i], 0) {
			beta[i] = 0
		}
	}
	return beta
}

func gaussianBlurGrid(src []float64, width, height int, sigma float64) []float64 {
	if sigma <= 0 || width < 2 || height < 2 {
		out := make([]float64, len(src))
		copy(out, src)
		return out
	}
	radius := int(math.Ceil(2 * sigma))
	if radius < 1 {
		radius = 1
	}
	kernel := make([]float64, radius+1)
	sum := 0.0
	twoSigma2 := 2 * sigma * sigma
	for i := 0; i <= radius; i++ {
		kernel[i] = math.Exp(-float64(i*i) / twoSigma2)
		if i == 0 {
			sum += kernel[i]
		} else {
			sum += 2 * kernel[i]
		}
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	tmp := make([]float64, len(src))
	dst := make([]float64, len(src))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := kernel[0] * src[y*width+x]
			for k := 1; k <= radius; k++ {
				xm := x - k
				xp := x + k
				if xm < 0 {
					xm = 0
				}
				if xp >= width {
					xp = width - 1
				}
				v += kernel[k] * (src[y*width+xm] + src[y*width+xp])
			}
			tmp[y*width+x] = v
		}
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := kernel[0] * tmp[y*width+x]
			for k := 1; k <= radius; k++ {
				ym := y - k
				yp := y + k
				if ym < 0 {
					ym = 0
				}
				if yp >= height {
					yp = height - 1
				}
				v += kernel[k] * (tmp[ym*width+x] + tmp[yp*width+x])
			}
			dst[y*width+x] = v
		}
	}
	return dst
}
