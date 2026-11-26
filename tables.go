// Package observermap implements an EXPERIMENTAL empirical mapping between
// the CIE 1931 2-degree and CIE 1964 10-degree standard observers.
//
// Nothing in this package is a CIE or ICC standard conversion. A deterministic
// least-squares mapping fitted on synthetic smooth reflectance spectra is used,
// and every result derived from it is labelled approximate/experimental.
package observermap

import (
	"embed"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
)

//go:embed data/*.csv
var dataFS embed.FS

// Observer identifies a CIE standard observer table.
type Observer string

const (
	// Observer1931 is the CIE 1931 2 degree standard observer (xbar,ybar,zbar).
	Observer1931 Observer = "CIE 1931 2° Standard Observer"
	// Observer1964 is the CIE 1964 10 degree supplementary standard observer.
	Observer1964 Observer = "CIE 1964 10° Supplementary Standard Observer"
)

// CMFTable holds a color matching function table on a common wavelength grid.
type CMFTable struct {
	Observer   Observer
	Wavelength []float64
	Xbar       []float64
	Ybar       []float64
	Zbar       []float64
}

// Illuminant is a spectral power distribution on a wavelength grid.
type Illuminant struct {
	Name       string
	Wavelength []float64
	SPD        []float64
}

func loadCSVRows(name string) ([][]float64, error) {
	f, err := dataFS.Open("data/" + name)
	if err != nil {
		return nil, fmt.Errorf("open embedded %s: %w", name, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil { // header
		return nil, fmt.Errorf("read header %s: %w", name, err)
	}
	var rows [][]float64
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		row := make([]float64, len(rec))
		for i, cell := range rec {
			v, err := strconv.ParseFloat(cell, 64)
			if err != nil {
				return nil, fmt.Errorf("parse %s cell %q: %w", name, cell, err)
			}
			row[i] = v
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("embedded table %s is empty", name)
	}
	return rows, nil
}

// LoadCMF loads the embedded 5 nm CMF table (380-780 nm) for the observer.
func LoadCMF(observer Observer) (*CMFTable, error) {
	file := ""
	switch observer {
	case Observer1931:
		file = "cie1931_2deg_5nm.csv"
	case Observer1964:
		file = "cie1964_10deg_5nm.csv"
	default:
		return nil, fmt.Errorf("unknown observer %q", observer)
	}
	rows, err := loadCSVRows(file)
	if err != nil {
		return nil, err
	}
	t := &CMFTable{Observer: observer}
	for _, row := range rows {
		if len(row) != 4 {
			return nil, fmt.Errorf("%s: expected 4 columns, got %d", file, len(row))
		}
		t.Wavelength = append(t.Wavelength, row[0])
		t.Xbar = append(t.Xbar, row[1])
		t.Ybar = append(t.Ybar, row[2])
		t.Zbar = append(t.Zbar, row[3])
	}
	return t, nil
}

// LoadIlluminant loads a named embedded illuminant CSV.
func LoadIlluminant(file, name string) (*Illuminant, error) {
	rows, err := loadCSVRows(file)
	if err != nil {
		return nil, err
	}
	ill := &Illuminant{Name: name}
	for _, row := range rows {
		if len(row) != 2 {
			return nil, fmt.Errorf("%s: expected 2 columns, got %d", file, len(row))
		}
		ill.Wavelength = append(ill.Wavelength, row[0])
		ill.SPD = append(ill.SPD, row[1])
	}
	return ill, nil
}

// WhitePoint is a reference white in XYZ (Y scale 0..100).
type WhitePoint struct {
	Name     string
	Observer Observer
	XYZ      XYZ
}

// IlluminantSetup bundles an illuminant with the white points obtained by
// integrating a perfect reflector under each observer.
type IlluminantSetup struct {
	Ill     *Illuminant
	White2  WhitePoint
	White10 WhitePoint
}

// StandardData bundles the embedded observer/illuminant tables and the white
// points obtained by integrating a perfect reflector under D50 and D65.
type StandardData struct {
	CMF2  *CMFTable
	CMF10 *CMFTable
	D65   IlluminantSetup
	D50   IlluminantSetup
}

// LoadStandardData loads every embedded table and derives D65 white points.
func LoadStandardData() (*StandardData, error) {
	cmf2, err := LoadCMF(Observer1931)
	if err != nil {
		return nil, err
	}
	cmf10, err := LoadCMF(Observer1964)
	if err != nil {
		return nil, err
	}
	d65, err := LoadIlluminant("d65_illuminant_5nm.csv", "CIE D65")
	if err != nil {
		return nil, err
	}
	d50, err := LoadIlluminant("d50_illuminant_5nm.csv", "CIE D50")
	if err != nil {
		return nil, err
	}
	d := &StandardData{CMF2: cmf2, CMF10: cmf10}
	d.D65 = IlluminantSetup{
		Ill:     d65,
		White2:  WhitePoint{Name: "D65", Observer: Observer1931, XYZ: IntegrateReflectanceXYZ(cmf2, d65, nil)},
		White10: WhitePoint{Name: "D65", Observer: Observer1964, XYZ: IntegrateReflectanceXYZ(cmf10, d65, nil)},
	}
	d.D50 = IlluminantSetup{
		Ill:     d50,
		White2:  WhitePoint{Name: "D50", Observer: Observer1931, XYZ: IntegrateReflectanceXYZ(cmf2, d50, nil)},
		White10: WhitePoint{Name: "D50", Observer: Observer1964, XYZ: IntegrateReflectanceXYZ(cmf10, d50, nil)},
	}
	return d, nil
}

func sameGrid(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			return false
		}
	}
	return true
}
