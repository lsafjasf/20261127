package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"lightanalyze/lighting"
)

func writeCSV(path string, curves lighting.Curves) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"time_s", "global_luma", "stable_background_luma",
		"stable_background_fraction", "foreground_luma",
		"foreground_motion_fraction", "sustained_luma",
		"sustained_background_luma", "fast_residual_luma",
	}
	for i := range curves.RegionLuma {
		header = append(header, fmt.Sprintf("stable_region_%d_luma", i))
	}
	if err := w.Write(header); err != nil {
		return err
	}
	n := len(curves.TimeS)
	for i := 0; i < n; i++ {
		row := []string{
			formatFloat(at(curves.TimeS, i)),
			formatFloat(at(curves.GlobalLuma, i)),
			formatFloat(at(curves.StableBackgroundLuma, i)),
			formatFloat(at(curves.StableBackgroundFraction, i)),
			formatFloat(at(curves.ForegroundLuma, i)),
			formatFloat(at(curves.ForegroundMotion, i)),
			formatFloat(at(curves.SustainedLuma, i)),
			formatFloat(at(curves.SustainedBackgroundLuma, i)),
			formatFloat(at(curves.FastResidualLuma, i)),
		}
		for _, series := range curves.RegionLuma {
			row = append(row, formatFloat(at(series, i)))
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func at(values []float64, i int) float64 {
	if i < 0 || i >= len(values) {
		return 0
	}
	return values[i]
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}
