package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"flatfield"
)

type options struct {
	flat         string
	applyTable   string
	dark         string
	lensTable    string
	inImage      string
	outImage     string
	tableOut     string
	csvOut       string
	reportOut    string
	sceneGradOut string
	kind         string
	reference    string
	gridW        int
	gridH        int
	centerX      float64
	centerY      float64
	aspect       float64
	sigma        float64
	maxGain      float64
	csvWidth     int
	csvHeight    int
	jpegQuality  int
	quiet        bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "flatfield: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var o options
	fs := flag.NewFlagSet("flatfield", flag.ContinueOnError)
	fs.StringVar(&o.flat, "flat", "", "uniform flat-field image (PNG/JPEG/GIF first frame)")
	fs.StringVar(&o.applyTable, "apply-table", "", "existing correction JSON to apply or resample; skips -flat estimation")
	fs.StringVar(&o.dark, "dark", "", "optional dark frame with same dimensions")
	fs.StringVar(&o.lensTable, "lens-table", "", "optional independently measured lens JSON table")
	fs.StringVar(&o.inImage, "in", "", "optional scene image to correct")
	fs.StringVar(&o.outImage, "out", "", "corrected image output path (PNG/JPEG)")
	fs.StringVar(&o.tableOut, "table-out", "", "correction table JSON output (default flatfield-table.json with -flat)")
	fs.StringVar(&o.csvOut, "csv-out", "", "optional gain matrix CSV output")
	fs.StringVar(&o.reportOut, "report-out", "", "optional text report output path")
	fs.StringVar(&o.sceneGradOut, "scene-gradient-out", "", "optional JSON affine-gradient diagnostic for -in; never changes correction")
	fs.StringVar(&o.kind, "kind", "device", "primary table: device|device_no_gradient|panel|panel_residual|lens|observed|gradient")
	fs.StringVar(&o.reference, "reference", "center", "normalization: center|geometric_mean|maximum")
	fs.IntVar(&o.gridW, "grid-w", 32, "table grid width")
	fs.IntVar(&o.gridH, "grid-h", 0, "table grid height; 0 derives from aspect ratio")
	fs.Float64Var(&o.centerX, "cx", 0.5, "optical center x in normalized coordinates")
	fs.Float64Var(&o.centerY, "cy", 0.5, "optical center y in normalized coordinates")
	fs.Float64Var(&o.aspect, "aspect", 0, "aspect ratio override; 0 uses flat image")
	fs.Float64Var(&o.sigma, "sigma", 0.8, "log-field Gaussian smoothing sigma in cells; -1 disables")
	fs.Float64Var(&o.maxGain, "max-gain", 16, "maximum brightening gain; -1 disables")
	fs.IntVar(&o.csvWidth, "csv-width", 0, "optional exact CSV resample width")
	fs.IntVar(&o.csvHeight, "csv-height", 0, "optional exact CSV resample height")
	fs.IntVar(&o.jpegQuality, "jpeg-quality", 95, "JPEG output quality")
	fs.BoolVar(&o.quiet, "quiet", false, "do not print report to stdout")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if o.applyTable != "" && o.flat != "" {
		return fmt.Errorf("-apply-table and -flat cannot be used together")
	}
	if o.applyTable == "" && o.flat == "" {
		fs.Usage()
		return fmt.Errorf("-flat or -apply-table is required")
	}
	if (o.inImage == "") != (o.outImage == "") {
		return fmt.Errorf("-in and -out must be supplied together")
	}
	if o.sceneGradOut != "" && o.inImage == "" {
		return fmt.Errorf("-scene-gradient-out requires -in")
	}
	if o.applyTable == "" && o.tableOut == "" {
		o.tableOut = "flatfield-table.json"
	}

	if o.applyTable != "" {
		table, err := flatfield.LoadTable(o.applyTable)
		if err != nil {
			return fmt.Errorf("load correction table: %w", err)
		}
		return outputAll(o, table)
	}

	flat, err := flatfield.LoadRaster(o.flat)
	if err != nil {
		return fmt.Errorf("load flat: %w", err)
	}
	estOpts := flatfield.DefaultOptions()
	estOpts.GridWidth = o.gridW
	estOpts.GridHeight = o.gridH
	estOpts.OpticalCenterX = o.centerX
	estOpts.OpticalCenterY = o.centerY
	estOpts.AspectRatio = o.aspect
	estOpts.SmoothSigma = o.sigma
	estOpts.MaxGain = o.maxGain
	estOpts.Kind = flatfield.Kind(o.kind)
	estOpts.Reference = flatfield.Reference(o.reference)

	if o.lensTable != "" {
		lens, err := flatfield.LoadTable(o.lensTable)
		if err != nil {
			return fmt.Errorf("load lens table: %w", err)
		}
		estOpts.Lens = lens
	}
	if o.dark != "" {
		dark, err := flatfield.LoadRaster(o.dark)
		if err != nil {
			return fmt.Errorf("load dark: %w", err)
		}
		table, err := flatfield.Estimate(flat, dark, estOpts)
		if err != nil {
			return err
		}
		return outputAll(o, table)
	}

	table, err := flatfield.Estimate(flat, nil, estOpts)
	if err != nil {
		return err
	}
	return outputAll(o, table)
}

func outputAll(o options, table *flatfield.Table) error {
	if o.tableOut != "" {
		if err := table.SaveJSON(o.tableOut); err != nil {
			return fmt.Errorf("save table: %w", err)
		}
	}
	if o.csvOut != "" {
		var err error
		if o.csvWidth > 0 && o.csvHeight > 0 {
			err = table.SaveResampledGainCSV(o.csvOut, o.csvWidth, o.csvHeight)
		} else {
			err = table.WriteGainCSV(o.csvOut)
		}
		if err != nil {
			return fmt.Errorf("save CSV: %w", err)
		}
	}
	report := table.SummaryText()
	if o.reportOut != "" {
		if err := os.MkdirAll(filepath.Dir(o.reportOut), 0o755); err != nil {
			return fmt.Errorf("create report directory: %w", err)
		}
		if err := os.WriteFile(o.reportOut, []byte(report), 0o644); err != nil {
			return fmt.Errorf("save report: %w", err)
		}
	}
	if o.inImage != "" {
		img, err := flatfield.LoadImage(o.inImage)
		if err != nil {
			return fmt.Errorf("load scene: %w", err)
		}
		if o.sceneGradOut != "" {
			gradient, err := flatfield.EstimateSceneGradient(flatfield.ImageRaster{Image: img}, 0, 0)
			if err != nil {
				return fmt.Errorf("estimate scene gradient: %w", err)
			}
			data, err := json.MarshalIndent(gradient, "", "  ")
			if err != nil {
				return fmt.Errorf("encode scene gradient: %w", err)
			}
			data = append(data, '\n')
			if err := os.MkdirAll(filepath.Dir(o.sceneGradOut), 0o755); err != nil {
				return fmt.Errorf("create scene gradient directory: %w", err)
			}
			if err := os.WriteFile(o.sceneGradOut, data, 0o644); err != nil {
				return fmt.Errorf("save scene gradient: %w", err)
			}
		}
		corrected, err := flatfield.CorrectImage(img, table)
		if err != nil {
			return fmt.Errorf("correct scene: %w", err)
		}
		if err := flatfield.SaveImage(o.outImage, corrected, o.jpegQuality); err != nil {
			return fmt.Errorf("save corrected image: %w", err)
		}
	}
	if !o.quiet {
		fmt.Print(report)
		fmt.Printf("table: %s\n", o.tableOut)
	}
	return nil
}
