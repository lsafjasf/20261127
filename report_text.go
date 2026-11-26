package main

import (
	"fmt"
	"strings"
)

func formatText(report *Report) string {
	var out strings.Builder

	fmt.Fprintf(&out, "CPL: %s\n", report.CPLPath)
	if report.CompositionEditRate != "" {
		fmt.Fprintf(&out, "Composition edit rate: %s\n", report.CompositionEditRate)
	}
	fmt.Fprintf(&out, "Assets: %d\n", report.AssetCount)
	fmt.Fprintf(&out, "Declared timeline duration: %s s\n", report.TotalSeconds)

	for _, asset := range report.Assets {
		fmt.Fprintf(&out, "\nReel %d · %s · %s\n", asset.ReelIndex, asset.Kind, shortID(asset.ID))
		fmt.Fprintf(&out, "  editRate=%s entryPoint=%d intrinsicDuration=%d duration=%d\n",
			asset.EditRate, asset.EntryPoint, asset.IntrinsicDuration, asset.Duration)
		fmt.Fprintf(&out, "  availableUnits=%d requiredEnd=%d status=%s\n",
			asset.AvailableUnits, asset.RequiredEnd, strings.ToUpper(asset.Status))

		if asset.FirstMissingOutputUnit >= 0 {
			fmt.Fprintf(&out, "  first missing: outputUnit=%d sourceFrame=%d overage=%d\n",
				asset.FirstMissingOutputUnit, asset.FirstMissingSource, asset.OverageUnits)
			fmt.Fprintf(&out, "  reelRelative=%ss absolute=%ss",
				asset.ReelRelativeSeconds, asset.AbsoluteSeconds)
			if asset.CPLFrame != "" {
				fmt.Fprintf(&out, " cplFrame=%s", asset.CPLFrame)
			}
			if asset.Timecode != "" {
				fmt.Fprintf(&out, " timecode=%s", asset.Timecode)
			}
			out.WriteByte('\n')
		}

		if asset.Reference != nil {
			fmt.Fprintf(&out, "  references: pkl=%t assetMap=%t\n",
				asset.Reference.InPKL, asset.Reference.InAssetMap)
			for _, path := range asset.Reference.Paths {
				state := "missing"
				if path.Exists {
					state = "readable"
				}
				fmt.Fprintf(&out, "    chunk %s [%s]", path.Path, state)
				if path.ExpectedSize > 0 {
					fmt.Fprintf(&out, " size=%d/%d ok=%t", path.ActualSize, path.ExpectedSize, path.SizeOK)
				}
				out.WriteByte('\n')
			}
		}

		for _, problem := range asset.Problems {
			fmt.Fprintf(&out, "  problem: %s\n", problem)
		}
	}

	out.WriteByte('\n')
	if report.FirstUnplayable != nil {
		moment := report.FirstUnplayable
		fmt.Fprintf(&out, "FIRST UNPLAYABLE: reel=%d asset=%s at %s s",
			moment.Reel, shortID(moment.AssetID), moment.Seconds)
		if moment.CPLFrame != "" {
			fmt.Fprintf(&out, " cplFrame=%s", moment.CPLFrame)
		}
		if moment.Timecode != "" {
			fmt.Fprintf(&out, " timecode=%s", moment.Timecode)
		}
		fmt.Fprintf(&out, " (source frame %d, %d short edit units)\n",
			moment.FirstMissingSource, moment.OverageUnits)
	} else {
		out.WriteString("FIRST UNPLAYABLE: none within CPL-declared source ranges\n")
	}

	for _, mismatch := range report.ReelDurationMismatches {
		fmt.Fprintf(&out, "duration mismatch: %s\n", mismatch)
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(&out, "warning: %s\n", warning)
	}
	for _, err := range report.Errors {
		fmt.Fprintf(&out, "error: %s\n", err)
	}

	return out.String()
}

func shortID(id string) string {
	if len(id) <= 13 {
		return id
	}
	return id[:8] + "…" + id[len(id)-4:]
}
