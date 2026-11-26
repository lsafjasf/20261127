package flatfield

import (
	"errors"
	"fmt"
	"math"
)

type cellSample struct {
	level  float64
	weight float64
	u, v   float64
	dx, dy float64
	r, r2  float64
}

// Estimate measures the two-dimensional response of a flat-field raster.
//
// If dark is non-nil it must have the same dimensions as flat and is subtracted
// before estimation. opts.Lens optionally supplies an independently measured
// lens table; without it, a centered radial model is attributed to lens
// vignetting and the remaining smooth field to panel non-uniformity.
func Estimate(flat Raster, dark Raster, opts Options) (*Table, error) {
	fw, fh, err := dims(flat)
	if err != nil {
		return nil, err
	}
	if dark != nil {
		dw, dh, err := dims(dark)
		if err != nil {
			return nil, err
		}
		if dw != fw || dh != fh {
			return nil, fmt.Errorf("flatfield: dark dimensions %dx%d do not match flat %dx%d", dw, dh, fw, fh)
		}
	}
	cfg, err := opts.withDefaults(fw, fh)
	if err != nil {
		return nil, err
	}

	gw, gh := cfg.GridWidth, cfg.GridHeight
	levels := make([]float64, gw*gh)
	weights := make([]float64, gw*gh)
	totalLevel, totalWeight := 0.0, 0.0
	nonPositive := 0

	for gy := 0; gy < gh; gy++ {
		y0 := gy * fh / gh
		y1 := (gy + 1) * fh / gh
		for gx := 0; gx < gw; gx++ {
			x0 := gx * fw / gw
			x1 := (gx + 1) * fw / gw
			sum, count := 0.0, 0.0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					c := flat.LinearRGBAt(x, y)
					s := luma(c)
					if dark != nil {
						s -= luma(dark.LinearRGBAt(x, y))
					}
					sum += s
					count++
				}
			}
			if count == 0 {
				continue
			}
			idx := gy*gw + gx
			levels[idx] = sum / count
			weights[idx] = count
			totalLevel += sum
			totalWeight += count
			if levels[idx] <= 1e-6 {
				nonPositive++
			}
		}
	}
	if totalWeight == 0 {
		return nil, errors.New("flatfield: flat field contains no samples")
	}
	meanLevel := totalLevel / totalWeight
	if meanLevel <= 1e-6 {
		return nil, errors.New("flatfield: flat field is too dark after dark subtraction")
	}

	floor := math.Max(meanLevel*1e-5, 1e-8)
	logLevels := make([]float64, len(levels))
	for i, v := range levels {
		if v < floor {
			v = floor
		}
		logLevels[i] = math.Log(v)
	}
	smoothed := gaussianBlurGrid(logLevels, gw, gh, cfg.SmoothSigma)

	samples := make([]cellSample, 0, gw*gh)
	rScale := 0.0
	corners := [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	for _, c := range corners {
		dx := (c[0] - cfg.OpticalCenterX) * cfg.AspectRatio
		dy := c[1] - cfg.OpticalCenterY
		rScale = math.Max(rScale, math.Hypot(dx, dy))
	}
	if rScale == 0 {
		rScale = 1
	}
	for gy := 0; gy < gh; gy++ {
		v := (float64(gy) + 0.5) / float64(gh)
		for gx := 0; gx < gw; gx++ {
			u := (float64(gx) + 0.5) / float64(gw)
			dx := (u - cfg.OpticalCenterX) * cfg.AspectRatio
			dy := (v - cfg.OpticalCenterY)
			r := math.Hypot(dx, dy) / rScale
			idx := gy*gw + gx
			samples = append(samples, cellSample{
				level: smoothed[idx], weight: weights[idx], u: u, v: v,
				dx: dx, dy: dy, r: r, r2: r * r,
			})
		}
	}

	refLog := referenceLog(smoothed, gw, gh, cfg)
	tab := &Table{
		Schema:                SchemaVersion,
		Kind:                  cfg.Kind,
		GridWidth:             gw,
		GridHeight:            gh,
		CoordinateSystem:      "normalized_pixel_centers",
		ToneDomain:            "linear_rgb",
		OpticalCenterX:        cfg.OpticalCenterX,
		OpticalCenterY:        cfg.OpticalCenterY,
		AspectRatio:           cfg.AspectRatio,
		Reference:             cfg.Reference,
		MaxGain:               cfg.MaxGain,
		ObservedResponse:      make([]float64, gw*gh),
		GradientResponse:      make([]float64, gw*gh),
		LensResponse:          make([]float64, gw*gh),
		PanelResponse:         make([]float64, gw*gh),
		PanelResidualResponse: make([]float64, gw*gh),
		Response:              make([]float64, gw*gh),
		Gain:                  make([]float64, gw*gh),
		Report: Report{
			SourceWidth: fw, SourceHeight: fh, GridWidth: gw, GridHeight: gh,
			MeanLinearLevel: meanLevel,
		},
	}
	if nonPositive > 0 {
		tab.Report.Warnings = append(tab.Report.Warnings,
			fmt.Sprintf("%d grid cells were at or below black and were floored before log fitting", nonPositive))
	}

	beta, lensLogs, lensReference, err := fitComponents(samples, smoothed, cfg)
	if err != nil {
		return nil, err
	}
	buildResponses(tab, samples, smoothed, refLog, beta, lensLogs)
	copy(tab.Response, tab.ObservedResponse)
	for i, r := range tab.ObservedResponse {
		tab.Gain[i] = 1 / r
	}
	if lensReference != "" {
		tab.Report.Lens.Reference = lensReference
	}
	buildReport(tab, samples, smoothed, weights, beta, cfg, lensReference != "")

	primary, err := tab.ForKind(cfg.Kind, cfg.MaxGain)
	if err != nil {
		return nil, err
	}
	*tab = *primary
	return tab, nil
}

func referenceLog(logLevels []float64, gw, gh int, cfg Options) float64 {
	switch cfg.Reference {
	case ReferenceGeometricMean:
		sum := 0.0
		for _, v := range logLevels {
			sum += v
		}
		return sum / float64(len(logLevels))
	case ReferenceMaximum:
		m := math.Inf(-1)
		for _, v := range logLevels {
			m = math.Max(m, v)
		}
		return m
	default:
		return sampleGridArray(logLevels, gw, gh, cfg.OpticalCenterX, cfg.OpticalCenterY)
	}
}
