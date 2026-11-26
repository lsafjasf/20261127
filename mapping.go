package observermap

import (
	"fmt"
	"math"
	"sort"
)

// EmpiricalMap is a least-squares 3x3 matrix M with
//
//	XYZ_10_hat = M * XYZ_2
//
// fitted on paired tristimulus values of synthetic reflectance spectra.
//
// It is an EMPIRICAL APPROXIMATION, not a CIE or ICC standard conversion.
// A deterministic 3-to-3 observer mapping cannot exist in general because
// different spectra can be metameric under the 1931 observer while producing
// different tristimulus values under the 1964 observer.
type EmpiricalMap struct {
	Matrix       [3][3]float64 `json:"matrix_row_major"`
	Direction    string        `json:"direction"`
	Method       string        `json:"method"`
	TrainSamples int           `json:"train_samples"`
	TestSamples  int           `json:"test_samples"`
	Quality      MapQuality    `json:"quality"`
}

// MapQuality summarizes held-out approximation errors of the empirical map.
type MapQuality struct {
	MeanDE00   float64 `json:"mean_de00"`
	MedianDE00 float64 `json:"median_de00"`
	P95DE00    float64 `json:"p95_de00"`
	MaxDE00    float64 `json:"max_de00"`
	MedianXY   float64 `json:"median_delta_xy"`
	P95XY      float64 `json:"p95_delta_xy"`
}

// Apply maps 1931 2-degree XYZ to an estimated 1964 10-degree XYZ.
func (m EmpiricalMap) Apply(c XYZ) XYZ {
	v := [3]float64{c.X, c.Y, c.Z}
	var out [3]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			out[i] += m.Matrix[i][j] * v[j]
		}
	}
	return XYZ{X: out[0], Y: out[1], Z: out[2]}
}

// Direction names the empirical mapping direction.
type Direction string

const (
	// Dir2To10 maps CIE 1931 2° XYZ -> estimated CIE 1964 10° XYZ.
	Dir2To10 Direction = "2to10"
	// Dir10To2 maps CIE 1964 10° XYZ -> estimated CIE 1931 2° XYZ.
	Dir10To2 Direction = "10to2"
)

// FitEmpiricalMap fits the 2° -> 10° least-squares map.
func FitEmpiricalMap(d *StandardData, setup IlluminantSetup, trainN, testN int) (*EmpiricalMap, error) {
	return fitMap(d, setup, trainN, testN, Dir2To10)
}

// FitReverseEmpiricalMap fits the 10° -> 2° least-squares map independently
// (OLS inverse fit, not the matrix inverse).
func FitReverseEmpiricalMap(d *StandardData, setup IlluminantSetup, trainN, testN int) (*EmpiricalMap, error) {
	return fitMap(d, setup, trainN, testN, Dir10To2)
}

func fitMap(d *StandardData, setup IlluminantSetup, trainN, testN int, dir Direction) (*EmpiricalMap, error) {
	wl := d.CMF2.Wavelength
	train := GenerateReflectances(trainN, 0x0B5E71, wl)
	test := GenerateReflectances(testN, 0x7E57, wl)

	pairs := make([][2]XYZ, 0, len(train))
	for _, s := range train {
		x2 := IntegrateReflectanceXYZ(d.CMF2, setup.Ill, s.Reflectance)
		x10 := IntegrateReflectanceXYZ(d.CMF10, setup.Ill, s.Reflectance)
		if dir == Dir10To2 {
			x2, x10 = x10, x2
		}
		pairs = append(pairs, [2]XYZ{x2, x10})
	}

	// Normal equations A = sum x2 x2^T, B = sum x10 x2^T.
	var a [3][3]float64
	var b [3][3]float64 // b[outputComponent][inputComponent]
	for _, p := range pairs {
		x := [3]float64{p[0].X, p[0].Y, p[0].Z}
		y := [3]float64{p[1].X, p[1].Y, p[1].Z}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				a[i][j] += x[i] * x[j]
			}
		}
		for o := 0; o < 3; o++ {
			for j := 0; j < 3; j++ {
				b[o][j] += y[o] * x[j]
			}
		}
	}
	// Mild scale-aware ridge for numerical robustness.
	tr := (a[0][0] + a[1][1] + a[2][2]) / 3
	ridge := 1e-10 * tr
	for i := 0; i < 3; i++ {
		a[i][i] += ridge
	}

	m := &EmpiricalMap{
		Direction:    string(dir),
		Method:       fmt.Sprintf("ordinary least squares 3x3 %s on synthetic %s reflectance pairs", dir, setup.Ill.Name),
		TrainSamples: trainN,
		TestSamples:  testN,
	}
	for o := 0; o < 3; o++ {
		row, err := solve3(a, b[o])
		if err != nil {
			return nil, fmt.Errorf("fit observer map row %d: %w", o, err)
		}
		m.Matrix[o] = row
	}

	q, err := m.validate(d, setup, test, dir)
	if err != nil {
		return nil, err
	}
	m.Quality = q
	return m, nil
}

func (m EmpiricalMap) validate(d *StandardData, setup IlluminantSetup, test []Spectrum, dir Direction) (MapQuality, error) {
	var des, dxys []float64
	for _, s := range test {
		x2 := IntegrateReflectanceXYZ(d.CMF2, setup.Ill, s.Reflectance)
		true10 := IntegrateReflectanceXYZ(d.CMF10, setup.Ill, s.Reflectance)
		src, dst, dstWhite := x2, true10, setup.White10
		if dir == Dir10To2 {
			src, dst, dstWhite = true10, x2, setup.White2
		}
		hat := m.Apply(src)

		lt := XYZToLab(dst, dstWhite)
		lh := XYZToLab(hat, dstWhite)
		des = append(des, DeltaE2000(lt, lh))

		xt, yt, err1 := dst.Chromaticity()
		xh, yh, err2 := hat.Chromaticity()
		if err1 == nil && err2 == nil {
			dxys = append(dxys, math.Hypot(xt-xh, yt-yh))
		}
	}
	sort.Float64s(des)
	sort.Float64s(dxys)

	q := MapQuality{
		MeanDE00:   mean(des),
		MedianDE00: percentileSorted(des, 0.50),
		P95DE00:    percentileSorted(des, 0.95),
		MaxDE00:    des[len(des)-1],
		MedianXY:   percentileSorted(dxys, 0.50),
		P95XY:      percentileSorted(dxys, 0.95),
	}
	return q, nil
}

func mean(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func percentileSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(math.Round(p * float64(len(sorted)-1)))
	return sorted[idx]
}

// solve3 solves A x = rhs for a 3x3 system with partial pivoting.
func solve3(a [3][3]float64, rhs [3]float64) ([3]float64, error) {
	const n = 3
	var m [3][4]float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			m[i][j] = a[i][j]
		}
		m[i][n] = rhs[i]
	}
	for col := 0; col < n; col++ {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][col]) < 1e-12 {
			return [3]float64{}, fmt.Errorf("singular matrix")
		}
		m[col], m[pivot] = m[pivot], m[col]
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := m[r][col] / m[col][col]
			for c := col; c <= n; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	var x [3]float64
	for i := 0; i < n; i++ {
		x[i] = m[i][n] / m[i][i]
	}
	return x, nil
}
