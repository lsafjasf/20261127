package flatfield

import "math"

// EstimateSceneGradient fits an affine log-luma plane to an ordinary scene.
//
// This is a diagnostic only. A real scene gradient cannot be algebraically
// separated from illumination without a flat-field/reference capture, and it
// must not be baked into a device correction table.
func EstimateSceneGradient(r Raster, gridWidth, gridHeight int) (GradientStats, error) {
	w, h, err := dims(r)
	if err != nil {
		return GradientStats{}, err
	}
	if gridWidth <= 0 {
		gridWidth = minInt(24, w)
	}
	if gridHeight <= 0 {
		gridHeight = int(math.Round(float64(gridWidth) * float64(h) / float64(w)))
	}
	if gridWidth > w {
		gridWidth = w
	}
	if gridHeight > h {
		gridHeight = h
	}
	if gridWidth < 2 || gridHeight < 2 {
		return GradientStats{}, errFitFailed
	}
	aspect := float64(w) / float64(h)
	type sample struct {
		z      float64
		w      float64
		dx, dy float64
	}
	samples := make([]sample, 0, gridWidth*gridHeight)
	for gy := 0; gy < gridHeight; gy++ {
		y0, y1 := gy*h/gridHeight, (gy+1)*h/gridHeight
		v := (float64(gy) + .5) / float64(gridHeight)
		for gx := 0; gx < gridWidth; gx++ {
			x0, x1 := gx*w/gridWidth, (gx+1)*w/gridWidth
			u := (float64(gx) + .5) / float64(gridWidth)
			sum, count := 0.0, 0.0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					sum += luma(r.LinearRGBAt(x, y))
					count++
				}
			}
			if count == 0 || sum/count <= 1e-8 {
				continue
			}
			samples = append(samples, sample{
				z: math.Log(sum / count), w: count,
				dx: (u - .5) * aspect, dy: v - .5,
			})
		}
	}
	xs := make([][]float64, len(samples))
	ys := make([]float64, len(samples))
	ws := make([]float64, len(samples))
	for i, s := range samples {
		xs[i] = []float64{1, s.dx, s.dy}
		ys[i], ws[i] = s.z, s.w
	}
	beta := solveWeightedLeastSquares(xs, ys, ws)
	if beta == nil {
		return GradientStats{}, errFitFailed
	}

	stat := GradientStats{
		LogSlopeX: beta[1], LogSlopeY: beta[2],
		MagnitudePerHeight: math.Hypot(beta[1], beta[2]),
		Significant:        math.Hypot(beta[1], beta[2]) >= 0.01,
		Interpretation:     "ordinary-scene affine gradient candidate; preserve by applying multiplicative device gain rather than subtracting this plane",
	}
	if stat.MagnitudePerHeight > 0 {
		stat.AngleDegrees = math.Atan2(beta[2], beta[1]) * 180 / math.Pi
	}
	wSum, zMean, pMean := 0.0, 0.0, 0.0
	pred := make([]float64, len(samples))
	for i, s := range samples {
		pred[i] = beta[0] + beta[1]*s.dx + beta[2]*s.dy
		wSum += s.w
		zMean += s.z * s.w
		pMean += pred[i] * s.w
	}
	zMean /= wSum
	pMean /= wSum
	cov, vz, vp := 0.0, 0.0, 0.0
	for i, s := range samples {
		zd, pd := s.z-zMean, pred[i]-pMean
		cov += s.w * zd * pd
		vz += s.w * zd * zd
		vp += s.w * pd * pd
	}
	if vz > 0 && vp > 0 {
		stat.RSquared = clampFloat(cov*cov/(vz*vp), 0, 1)
	}
	return stat, nil
}
