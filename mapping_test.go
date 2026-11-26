package observermap

import "testing"

func TestEmpiricalMapQualityAndLabels(t *testing.T) {
	sd, err := LoadStandardData()
	if err != nil {
		t.Fatal(err)
	}
	m, err := FitEmpiricalMap(sd, sd.D65, 512, 256)
	if err != nil {
		t.Fatal(err)
	}
	// Loose deterministic bounds on the synthetic held-out population.
	if m.Quality.MedianDE00 > 2.0 {
		t.Errorf("median ΔE00 %.3f unexpectedly large", m.Quality.MedianDE00)
	}
	if m.Quality.P95DE00 > 6.0 {
		t.Errorf("p95 ΔE00 %.3f unexpectedly large", m.Quality.P95DE00)
	}

	// Mapping the 2° white must stay close to the 10° white chromaticity.
	hatWhite := m.Apply(sd.D65.White2.XYZ)
	hx, hy, err := hatWhite.Chromaticity()
	if err != nil {
		t.Fatal(err)
	}
	wx, wy, _ := sd.D65.White10.XYZ.Chromaticity()
	approx(t, hx, wx, 0.004, "mapped white x")
	approx(t, hy, wy, 0.004, "mapped white y")
}

func TestSolve3(t *testing.T) {
	a := [3][3]float64{{2, 1, -1}, {-3, -1, 2}, {-2, 1, 2}}
	rhs := [3]float64{8, -11, -3}
	x, err := solve3(a, rhs)
	if err != nil {
		t.Fatal(err)
	}
	// Known solution (2,3,-1).
	want := [3]float64{2, 3, -1}
	for i := range x {
		approx(t, x[i], want[i], 1e-9, "solve3 component")
	}
}

func TestDemoBuildsAndFlagsUncertainty(t *testing.T) {
	rep, err := BuildDemo("")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Profile.Uncertain {
		t.Error("missing profile must make demo profile assessment uncertain")
	}
	if len(rep.Spectral) == 0 || len(rep.Swatches) == 0 {
		t.Fatal("demo has no rows")
	}
	if !rep.Swatches[0].Uncertain {
		t.Error("swatch rows must inherit missing-profile uncertainty")
	}

	rep2, err := BuildDemo("testdata/monitor-d65.icc")
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Profile.Uncertain || !rep2.Profile.WhiteSupported {
		t.Errorf("D65 fixture should be supported: %+v", rep2.Profile)
	}
	for _, s := range rep2.Swatches {
		if s.Uncertain {
			t.Errorf("swatch %s should not be hard-uncertain with supported profile", s.Name)
		}
	}
}
