package main

import (
	"encoding/json"
	"fmt"
	"io"

	cc "github.com/lsafjasf/20261127"
)

func writeJSON(out io.Writer, analysis cc.Analysis) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(analysis)
}

func writeTable(out io.Writer, analysis cc.Analysis) {
	fmt.Fprintln(out, "ID                 RGB-E   RGBrank   Lab-E   DE2000   DErank  dominant        dL*       dC*       dh(deg)   Lterm     Cterm     Hterm     rotation")
	for _, sample := range analysis.Samples {
		components := sample.Components
		fmt.Fprintf(out,
			"%-18s %7.3f   %6d  %7.3f   %7.4f   %6d  %-10s %8.4f  %9.4f  %9.4f  %8.4f  %8.4f  %8.4f  %9.4f\n",
			sample.ID,
			sample.EuclideanRGB,
			sample.EuclideanRank,
			sample.EuclideanLab,
			sample.DeltaE2000,
			sample.DeltaE2000Rank,
			sample.Dominant,
			components.DeltaL,
			components.DeltaC,
			components.DeltaHueAngle,
			components.LightnessTerm,
			components.ChromaTerm,
			components.HueTerm,
			components.RotationTerm,
		)
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "RGB Euclidean order:   %v\n", analysis.EuclideanOrder)
	fmt.Fprintf(out, "CIEDE2000 order:       %v\n", analysis.DeltaE2000Order)
	if len(analysis.Reversals) == 0 {
		fmt.Fprintln(out, "Ordering reversals:    none")
		return
	}
	fmt.Fprintf(out, "Ordering reversals:    %d\n", len(analysis.Reversals))
	for _, reversal := range analysis.Reversals {
		fmt.Fprintf(out, "  %s vs %s: RGB closer=%s, CIEDE2000 closer=%s\n",
			reversal.SampleA,
			reversal.SampleB,
			reversal.EuclideanCloser,
			reversal.DeltaE2000Closer,
		)
	}
}
