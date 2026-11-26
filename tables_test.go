package observermap

import (
	"testing"
)

func TestEmbeddedTablesShape(t *testing.T) {
	sd, err := LoadStandardData()
	if err != nil {
		t.Fatal(err)
	}
	if len(sd.CMF2.Wavelength) != 81 || len(sd.CMF10.Wavelength) != 81 {
		t.Fatalf("expected 81 wavelength rows, got %d/%d", len(sd.CMF2.Wavelength), len(sd.CMF10.Wavelength))
	}
	if !sameGrid(sd.CMF2.Wavelength, sd.CMF10.Wavelength) ||
		!sameGrid(sd.CMF2.Wavelength, sd.D65.Ill.Wavelength) ||
		!sameGrid(sd.CMF2.Wavelength, sd.D50.Ill.Wavelength) {
		t.Fatal("embedded wavelength grids are not aligned")
	}
	if sd.CMF2.Wavelength[0] != 380 || sd.CMF2.Wavelength[80] != 780 {
		t.Fatalf("unexpected range: %v..%v", sd.CMF2.Wavelength[0], sd.CMF2.Wavelength[80])
	}
}

func TestIntegratedWhitePointsMatchCIETabulations(t *testing.T) {
	sd, err := LoadStandardData()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		w    WhitePoint
		x, y float64
	}{
		{"D65 2°", sd.D65.White2, 0.3127, 0.3290},
		{"D65 10°", sd.D65.White10, 0.3138, 0.3310},
		{"D50 2°", sd.D50.White2, 0.3457, 0.3585},
		{"D50 10°", sd.D50.White10, 0.3477, 0.3595},
	}
	for _, c := range cases {
		x, y, err := c.w.XYZ.Chromaticity()
		if err != nil {
			t.Fatal(err)
		}
		approx(t, x, c.x, 0.0003, c.name+" x")
		approx(t, y, c.y, 0.0003, c.name+" y")
		approx(t, c.w.XYZ.Y, 100, 1e-9, c.name+" Y normalization")
	}
}

func TestPerfectAndGreyReflectance(t *testing.T) {
	sd, _ := LoadStandardData()
	white := IntegrateReflectanceXYZ(sd.CMF2, sd.D65.Ill, nil)
	approx(t, white.Y, 100, 1e-9, "perfect reflector Y")

	grey := make([]float64, len(sd.CMF2.Wavelength))
	for i := range grey {
		grey[i] = 0.5
	}
	g := IntegrateReflectanceXYZ(sd.CMF2, sd.D65.Ill, grey)
	approx(t, g.Y, 50, 1e-9, "50% grey Y")
}
