package observermap

import (
	"fmt"
	"math"
	"math/rand"
)

// Spectrum is a named reflectance spectrum sampled on the embedded grid.
// Reflectance values are nominally in [0,1].
type Spectrum struct {
	Name        string
	Reflectance []float64
	Provenance  string // "synthetic" or analytic description
}

// IntegrateReflectanceXYZ computes object-colour XYZ for a reflectance
// spectrum under an illuminant and observer.
//
// The normalization constant makes a perfect reflector (nil reflectance,
// equivalently all ones) have Y == 100. The 5 nm samples are summed with the
// uniform wavelength step. If refl is nil a perfect reflector is assumed.
func IntegrateReflectanceXYZ(cmf *CMFTable, illum *Illuminant, refl []float64) XYZ {
	if illum != nil && !sameGrid(cmf.Wavelength, illum.Wavelength) {
		// All embedded tables share a grid; mismatched external data is a
		// programming error in this package, so keep the zero result explicit
		// via the guarded path below.
		return XYZ{}
	}
	if refl != nil && len(refl) != len(cmf.Wavelength) {
		return XYZ{}
	}

	step := cmf.Wavelength[1] - cmf.Wavelength[0]
	var sumX, sumY, sumZ, norm float64
	for i := range cmf.Wavelength {
		s := 1.0
		if illum != nil {
			s = illum.SPD[i]
		}
		r := 1.0
		if refl != nil {
			r = refl[i]
		}
		w := s * r * step
		sumX += w * cmf.Xbar[i]
		sumY += w * cmf.Ybar[i]
		sumZ += w * cmf.Zbar[i]
		norm += s * cmf.Ybar[i] * step
	}
	if norm == 0 {
		return XYZ{}
	}
	k := 100 / norm
	return XYZ{X: sumX * k, Y: sumY * k, Z: sumZ * k}
}

// gaussianBump returns a Gaussian centred at c with width sigma.
func gaussianBump(wl, c, sigma float64) float64 {
	d := (wl - c) / sigma
	return math.Exp(-0.5 * d * d)
}

// logisticEdge returns a smooth 0->1 edge centred at c.
func logisticEdge(wl, c, width float64) float64 {
	return 1 / (1 + math.Exp(-(wl-c)/width))
}

// AnalyticSpectra returns a small set of illustrative spectra designed to
// expose 2/10 degree differences: white, grey, narrow spectral bands and
// sharp reflectance edges. They are analytic constructions, not measurements.
func AnalyticSpectra(wavelengths []float64) []Spectrum {
	mk := func(name, provenance string, f func(w float64) float64) Spectrum {
		r := make([]float64, len(wavelengths))
		for i, w := range wavelengths {
			r[i] = math.Max(0, math.Min(1, f(w)))
		}
		return Spectrum{Name: name, Reflectance: r, Provenance: provenance}
	}
	return []Spectrum{
		mk("Perfect reflector", "r(lambda)=1", func(float64) float64 { return 1 }),
		mk("50% grey", "r(lambda)=0.5", func(float64) float64 { return 0.5 }),
		mk("Narrow blue band 470 nm", "Gaussian sigma=10 nm", func(w float64) float64 {
			return gaussianBump(w, 470, 10)
		}),
		mk("Narrow green band 520 nm", "Gaussian sigma=10 nm", func(w float64) float64 {
			return gaussianBump(w, 520, 10)
		}),
		mk("Narrow amber band 580 nm", "Gaussian sigma=8 nm", func(w float64) float64 {
			return gaussianBump(w, 580, 8)
		}),
		mk("Narrow red band 610 nm", "Gaussian sigma=10 nm", func(w float64) float64 {
			return gaussianBump(w, 610, 10)
		}),
		mk("Blue/green edge 500 nm", "logistic edge width=8 nm", func(w float64) float64 {
			return logisticEdge(w, 500, 8)
		}),
		mk("Amber/red edge 590 nm", "1-logistic edge width=8 nm", func(w float64) float64 {
			return 1 - logisticEdge(w, 590, 8)
		}),
	}
}

// GenerateReflectances builds a deterministic population of smooth,
// physically-plausible reflectance spectra (baseline plus random broad
// Gaussian bands and logistic edges), clipped to [0,1].
//
// These are synthetic training samples; they are not a library of real
// materials, which is one reason the fitted observer map stays experimental.
func GenerateReflectances(n int, seed int64, wavelengths []float64) []Spectrum {
	rng := rand.New(rand.NewSource(seed))
	out := make([]Spectrum, 0, n)
	for idx := 0; idx < n; idx++ {
		r := make([]float64, len(wavelengths))
		base := rng.Float64() * 0.25
		for i := range r {
			r[i] = base
		}

		bands := 1 + rng.Intn(4)
		for b := 0; b < bands; b++ {
			c := 400 + rng.Float64()*360
			sigma := 12 + rng.Float64()*110
			amp := 0.15 + rng.Float64()*0.75
			if rng.Float64() < 0.2 {
				amp = -amp * 0.7 // absorption band
			}
			for i, w := range wavelengths {
				r[i] += amp * gaussianBump(w, c, sigma)
			}
		}
		if rng.Float64() < 0.5 {
			edge := 430 + rng.Float64()*300
			width := 4 + rng.Float64()*40
			amp := 0.3 + rng.Float64()*0.6
			up := rng.Float64() < 0.5
			for i, w := range wavelengths {
				e := logisticEdge(w, edge, width)
				if !up {
					e = 1 - e
				}
				r[i] += amp * (e - 0.5)
			}
		}
		for i := range r {
			r[i] = math.Max(0, math.Min(1, r[i]))
		}
		out = append(out, Spectrum{
			Name:        fmt.Sprintf("synthetic-%04d", idx),
			Reflectance: r,
			Provenance:  fmt.Sprintf("seeded random smooth reflectance (seed=%d)", seed),
		})
	}
	return out
}
