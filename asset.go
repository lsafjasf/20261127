package main

import (
	"math/big"
	"strconv"
)

func analyzeAsset(pkg *Package, node *xmlNode, reelIndex int, compositionRate Rational, reelStart *big.Rat) AssetReport {
	id := normalizeID(childTextAny(node, "Id", "ID", "TrackFileId"))
	kind := node.localName()

	report := AssetReport{
		ReelIndex:              reelIndex + 1,
		Kind:                   kind,
		ID:                     id,
		EntryPoint:             0,
		FirstMissingOutputUnit: -1,
		Status:                 "ok",
	}

	if id == "" {
		report.Problems = append(report.Problems, "missing asset Id")
	} else {
		ref := checkReference(pkg, id)
		report.Reference = &ref
		if len(ref.Problems) > 0 {
			report.Problems = append(report.Problems, ref.Problems...)
		}
	}

	rate := compositionRate
	if raw := node.childText("EditRate"); raw != "" {
		parsed, err := parseRational(raw)
		if err != nil {
			report.Problems = append(report.Problems, "invalid EditRate: "+err.Error())
			report.Status = "invalid"
		} else {
			rate = parsed
		}
	}
	if rate.valid() {
		report.EditRate = strconv.FormatInt(rate.Num, 10) + " " + strconv.FormatInt(rate.Den, 10)
	} else if report.EditRate == "" {
		report.Problems = append(report.Problems, "missing or invalid EditRate")
		report.Status = "invalid"
	}

	intrinsic, hasIntrinsic, err := node.childInt("IntrinsicDuration")
	if err != nil {
		report.Problems = append(report.Problems, err.Error())
		report.Status = "invalid"
	} else if !hasIntrinsic {
		report.Problems = append(report.Problems, "missing IntrinsicDuration")
		report.Status = "invalid"
	} else {
		report.IntrinsicDuration = intrinsic
	}

	entry, hasEntry, err := node.childInt("EntryPoint")
	if err != nil {
		report.Problems = append(report.Problems, err.Error())
		report.Status = "invalid"
	} else if hasEntry {
		report.EntryPoint = entry
	}

	duration, hasDuration, err := node.childInt("Duration")
	if err != nil {
		report.Problems = append(report.Problems, err.Error())
		report.Status = "invalid"
	} else if hasDuration {
		report.Duration = duration
	} else if hasIntrinsic && err == nil {
		report.Duration = report.IntrinsicDuration - report.EntryPoint
	}

	if report.EntryPoint < 0 {
		report.Problems = append(report.Problems, "EntryPoint is negative")
		report.Status = "invalid"
	}
	if hasIntrinsic && report.IntrinsicDuration <= 0 {
		report.Problems = append(report.Problems, "IntrinsicDuration must be positive")
		report.Status = "invalid"
	}
	if hasDuration && report.Duration < 0 {
		report.Problems = append(report.Problems, "Duration is negative")
		report.Status = "invalid"
	}

	if report.Status == "invalid" || !hasIntrinsic || intrinsic <= 0 || report.EntryPoint < 0 {
		return report
	}

	report.RequiredEnd = report.EntryPoint + report.Duration
	if report.EntryPoint > report.IntrinsicDuration {
		report.AvailableUnits = 0
		report.OverageUnits = report.RequiredEnd - report.IntrinsicDuration
		report.FirstMissingOutputUnit = 0
		report.FirstMissingSource = report.EntryPoint
		report.Status = "invalid"
	} else {
		report.AvailableUnits = report.IntrinsicDuration - report.EntryPoint
		if report.Duration > report.AvailableUnits {
			report.OverageUnits = report.Duration - report.AvailableUnits
			report.FirstMissingOutputUnit = report.AvailableUnits
			report.FirstMissingSource = report.IntrinsicDuration
			report.Status = "short"
		}
	}

	if report.FirstMissingOutputUnit >= 0 && rate.valid() {
		relative := rate.unitsToSeconds(report.FirstMissingOutputUnit)
		absolute := new(big.Rat).Add(new(big.Rat).Set(reelStart), relative)
		report.ReelRelativeSeconds = formatRat(relative, 9)
		report.AbsoluteSeconds = formatRat(absolute, 9)
		report.momentSeconds = absolute

		frame := rationalFrame(absolute, compositionRate)
		if frame != nil {
			report.CPLFrame = frame.RatString()
			if compositionRate.Num%compositionRate.Den == 0 {
				fps := compositionRate.Num / compositionRate.Den
				report.Timecode = timecode(absolute, fps)
			}
		}
	}

	return report
}

func momentFromAsset(asset AssetReport, seconds *big.Rat, rate Rational) *Moment {
	moment := &Moment{
		Reel:               asset.ReelIndex,
		AssetKind:          asset.Kind,
		AssetID:            asset.ID,
		Seconds:            formatRat(seconds, 9),
		CPLFrame:           asset.CPLFrame,
		Timecode:           asset.Timecode,
		OutputUnit:         asset.FirstMissingOutputUnit,
		FirstMissingSource: asset.FirstMissingSource,
		OverageUnits:       asset.OverageUnits,
	}
	if frame := rationalFrame(seconds, rate); frame != nil {
		moment.CPLFrame = frame.RatString()
	}
	return moment
}
