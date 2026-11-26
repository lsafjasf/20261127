package flatfield

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// WriteGainCSV writes the selected table gain grid as a matrix.
// Rows are v and columns are u; normalized coordinates are implied by grid size.
func (t *Table) WriteGainCSV(path string) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	record := make([]string, t.GridWidth)
	for y := 0; y < t.GridHeight; y++ {
		for x := 0; x < t.GridWidth; x++ {
			record[x] = strconv.FormatFloat(t.Gain[y*t.GridWidth+x], 'g', 8, 64)
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return f.Close()
}

// SaveResampledGainCSV writes a gain matrix for an exact pixel resolution.
func (t *Table) SaveResampledGainCSV(path string, width, height int) error {
	gains, err := t.ResampleGain(width, height)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	for y := 0; y < height; y++ {
		row := make([]string, width)
		for x := 0; x < width; x++ {
			row[x] = strconv.FormatFloat(gains[y*width+x], 'g', 8, 64)
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return f.Close()
}

// GainString is a small diagnostic helper.
func (t *Table) GainString() string {
	return fmt.Sprintf("%dx%d %s gain table", t.GridWidth, t.GridHeight, t.Kind)
}
