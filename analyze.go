package observermap

import (
	"fmt"
	"math"
)

// Disclaimer is the mandatory labelling for every report and result.
const Disclaimer = "EXPERIMENTAL EMPIRICAL MAPPING BETWEEN CIE 1931 2° AND CIE 1964 10° OBSERVERS. " +
	"NOT a CIE or ICC standard conversion; a unique 2°<->10° mapping does not exist because of observer metamerism."

// ColorRow is one sample with direct and/or estimated observer coordinates.
type ColorRow struct {
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Hex             string  `json:"hex,omitempty"`
	X2              float64 `json:"x_2deg"`
	Y2              float64 `json:"y_2deg"`
	X10             float64 `json:"x_10deg"`
	Y10             float64 `json:"y_10deg"`
	DeltaXY         float64 `json:"delta_xy"`
	DeltaE00        float64 `json:"delta_e00_observer_diff"`
	MapResidualDE00 float64 `json:"map_residual_de00,omitempty"`
	HasTrue10       bool    `json:"has_direct_10deg"`
	Uncertain       bool    `json:"uncertain"`
	Note            string  `json:"note,omitempty"`

	XYZ2  XYZ `json:"xyz_2deg"`
	XYZ10 XYZ `json:"xyz_10deg_direct_or_estimated"`
	Lab2  Lab `json:"lab_2deg"`
	Lab10 Lab `json:"lab_10deg"`
}

// DemoReport is the full showcase output, also used for JSON rendering.
type DemoReport struct {
	Disclaimer    string       `json:"disclaimer"`
	Profile       Certainty    `json:"profile"`
	MapD65        EmpiricalMap `json:"map_d65_2to10"`
	MapD65Reverse EmpiricalMap `json:"map_d65_10to2"`
	MapD50        EmpiricalMap `json:"map_d50_2to10"`
	MapD50Reverse EmpiricalMap `json:"map_d50_10to2"`
	Spectral      []ColorRow   `json:"spectral_direct"`
	Swatches      []ColorRow   `json:"swatches_empirical"`
}

// BuildDemo loads tables, fits both empirical maps and assembles all rows.
func BuildDemo(profilePath string) (*DemoReport, error) {
	sd, err := LoadStandardData()
	if err != nil {
		return nil, err
	}
	map65, err := FitEmpiricalMap(sd, sd.D65, 512, 256)
	if err != nil {
		return nil, err
	}
	map50, err := FitEmpiricalMap(sd, sd.D50, 512, 256)
	if err != nil {
		return nil, err
	}
	map65r, err := FitReverseEmpiricalMap(sd, sd.D65, 512, 256)
	if err != nil {
		return nil, err
	}
	map50r, err := FitReverseEmpiricalMap(sd, sd.D50, 512, 256)
	if err != nil {
		return nil, err
	}
	cert := AssessProfile(profilePath)

	rep := &DemoReport{
		Disclaimer:    Disclaimer,
		Profile:       cert,
		MapD65:        *map65,
		MapD65Reverse: *map65r,
		MapD50:        *map50,
		MapD50Reverse: *map50r,
	}

	for _, sp := range AnalyticSpectra(sd.CMF2.Wavelength) {
		x2 := IntegrateReflectanceXYZ(sd.CMF2, sd.D65.Ill, sp.Reflectance)
		x10 := IntegrateReflectanceXYZ(sd.CMF10, sd.D65.Ill, sp.Reflectance)
		hat10 := map65.Apply(x2)
		row, err := buildRow(sp.Name, "spectral-direct", "", x2, x10, true, sd.D65)
		if err != nil {
			continue
		}
		row.MapResidualDE00 = DeltaE2000(
			XYZToLab(x10, sd.D65.White10),
			XYZToLab(hat10, sd.D65.White10))
		row.Note = sp.Provenance
		rep.Spectral = append(rep.Spectral, row)
	}

	swatches := []struct{ name, hex string }{
		{"sRGB white", "#FFFFFF"},
		{"sRGB red", "#FF0000"},
		{"sRGB green", "#00FF00"},
		{"sRGB blue", "#0000FF"},
		{"cyan", "#00FFFF"},
		{"magenta", "#FF00FF"},
		{"yellow", "#FFFF00"},
		{"skin tone", "#D8A07A"},
		{"sky blue", "#5B8DD9"},
		{"foliage", "#3E7D3A"},
	}
	for _, sw := range swatches {
		r, g, b, err := ParseHexRGB(sw.hex)
		if err != nil {
			return nil, err
		}
		x2 := SRGBToXYZ(r, g, b)
		hat10 := map65.Apply(x2)
		row, err := buildRow(sw.name, "srgb-empirical", sw.hex, x2, hat10, false, sd.D65)
		if err != nil {
			continue
		}
		row.Uncertain = cert.Uncertain
		if cert.Uncertain {
			row.Note = "UNCERTAIN: " + uncertaintySummary(cert)
		} else if cert.Profile != nil && cert.WhiteSupported && cert.WhitePoint != "D65" {
			row.Note = "empirical sRGB/D65 estimate; profile media white verified as " + cert.WhitePoint + " (not applied to sRGB)"
		} else {
			row.Note = "empirical estimate; profile white point " + cert.WhitePoint
		}
		rep.Swatches = append(rep.Swatches, row)
	}
	return rep, nil
}

func uncertaintySummary(c Certainty) string {
	if len(c.Reasons) > 0 {
		return c.Reasons[0]
	}
	return "provenance incomplete"
}

func buildRow(name, kind, hex string, x2, x10 XYZ, direct bool, setup IlluminantSetup) (ColorRow, error) {
	x2c, y2c, err := x2.Chromaticity()
	if err != nil {
		return ColorRow{}, fmt.Errorf("%s: %w", name, err)
	}
	x10c, y10c, err := x10.Chromaticity()
	if err != nil {
		return ColorRow{}, fmt.Errorf("%s: %w", name, err)
	}
	lab2 := XYZToLab(x2, setup.White2)
	lab10 := XYZToLab(x10, setup.White10)
	return ColorRow{
		Name:      name,
		Kind:      kind,
		Hex:       hex,
		X2:        x2c,
		Y2:        y2c,
		X10:       x10c,
		Y10:       y10c,
		DeltaXY:   math.Hypot(x10c-x2c, y10c-y2c),
		DeltaE00:  DeltaE2000(lab2, lab10),
		HasTrue10: direct,
		XYZ2:      x2,
		XYZ10:     x10,
		Lab2:      lab2,
		Lab10:     lab10,
	}, nil
}

// MapResult is the answer to a one-off observer mapping query.
type MapResult struct {
	Disclaimer string    `json:"disclaimer"`
	Input      string    `json:"input"`
	Direction  string    `json:"direction"`
	WhitePoint string    `json:"white_point"`
	Profile    Certainty `json:"profile"`
	Empirical  bool      `json:"empirical"`
	Standard   bool      `json:"standard_conversion"`
	ColorRow
	MapQuality MapQuality `json:"map_quality"`
}

// MapQuery runs a one-off XYZ or hex observer mapping.
//
// inputKind is "hex" (input like "#rrggbb") or "xyz" with xyz set.
// dir is "2to10" (default) or "10to2" (xyz inputs only).
// wp is the assumed white point ("D65" or "D50") when no supported profile
// overrides it. profilePath may be empty.
func MapQuery(inputKind, input, dir, wp, profilePath string, xyz XYZ) (*MapResult, error) {
	if dir == "" {
		dir = string(Dir2To10)
	}
	if dir != string(Dir2To10) && dir != string(Dir10To2) {
		return nil, fmt.Errorf("unsupported direction %q (use 2to10 or 10to2)", dir)
	}
	if inputKind == "hex" && dir == string(Dir10To2) {
		return nil, fmt.Errorf("hex/sRGB only defines 2° XYZ; --dir 10to2 requires --x/--y/--z input")
	}
	sd, err := LoadStandardData()
	if err != nil {
		return nil, err
	}
	cert := AssessProfile(profilePath)

	wpName := wp
	// sRGB hex values are defined against D65 by IEC 61966-2-1, so the
	// mathematical white point is fixed for hex input. A supported profile
	// white point only overrides the assumption for raw XYZ queries (e.g.
	// ICC PCS/media-white XYZ, which is commonly D50).
	if inputKind == "hex" {
		if wpName != "D65" {
			cert.Reasons = append(cert.Reasons, "ignoring --wp for hex input: sRGB is fixed to D65")
		}
		wpName = "D65"
	} else if cert.ProfileReadable && cert.WhiteSupported {
		if cert.WhitePoint != wpName {
			cert.Reasons = append(cert.Reasons,
				fmt.Sprintf("profile white point %s overrides requested assumption %s", cert.WhitePoint, wpName))
		}
		wpName = cert.WhitePoint
	}
	if !cert.ProfilePresent || cert.ProfileError != "" {
		cert = AssessAssumedWhite(wpName, cert)
	}
	if wpName != "D50" && wpName != "D65" {
		cert.Uncertain = true
		cert.Reasons = append(cert.Reasons, fmt.Sprintf("unsupported white point %q", wpName))
	}
	if inputKind == "hex" && cert.ProfileReadable && cert.WhiteSupported && cert.WhitePoint != "D65" {
		cert.Reasons = append(cert.Reasons,
			"hex/sRGB input stays D65-encoded; profile white point "+cert.WhitePoint+" concerns ICC XYZ/PCS values, not the sRGB conversion")
	}

	setup, mp, err := fitForWhiteDir(sd, wpName, dir)
	if err != nil {
		return nil, err
	}

	var src XYZ
	label := input
	switch inputKind {
	case "hex":
		r, g, b, err := ParseHexRGB(input)
		if err != nil {
			return nil, err
		}
		src = SRGBToXYZ(r, g, b)
	case "xyz":
		src = xyz
		label = fmt.Sprintf("XYZ(%.3f,%.3f,%.3f)", xyz.X, xyz.Y, xyz.Z)
	default:
		return nil, fmt.Errorf("unknown input kind %q", inputKind)
	}

	hat := mp.Apply(src)
	var row ColorRow
	if dir == string(Dir2To10) {
		row, err = buildRow(label, inputKind+"-empirical-2to10", "", src, hat, false, setup)
	} else {
		// Row columns stay (2°,10°): estimated 2° first, source 10° second.
		row, err = buildRow(label, inputKind+"-empirical-10to2", "", hat, src, false, setup)
	}
	if err != nil {
		return nil, err
	}
	row.Uncertain = cert.Uncertain
	if cert.Uncertain {
		row.Note = "UNCERTAIN: " + uncertaintySummary(cert)
	}

	return &MapResult{
		Disclaimer: Disclaimer,
		Input:      label,
		Direction:  dir,
		WhitePoint: wpName,
		Profile:    cert,
		Empirical:  true,
		Standard:   false,
		ColorRow:   row,
		MapQuality: mp.Quality,
	}, nil
}

func fitForWhiteDir(sd *StandardData, wpName, dir string) (IlluminantSetup, *EmpiricalMap, error) {
	reverse := Direction(dir) == Dir10To2
	if wpName == "D50" {
		if reverse {
			m, err := FitReverseEmpiricalMap(sd, sd.D50, 512, 256)
			return sd.D50, m, err
		}
		m, err := FitEmpiricalMap(sd, sd.D50, 512, 256)
		return sd.D50, m, err
	}
	if reverse {
		m, err := FitReverseEmpiricalMap(sd, sd.D65, 512, 256)
		return sd.D65, m, err
	}
	m, err := FitEmpiricalMap(sd, sd.D65, 512, 256)
	return sd.D65, m, err
}
