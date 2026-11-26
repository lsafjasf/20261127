package main

import (
	"math/big"
	"strconv"
)

type Moment struct {
	Reel               int    `json:"reel"`
	AssetKind          string `json:"asset_kind"`
	AssetID            string `json:"asset_id"`
	Seconds            string `json:"seconds"`
	CPLFrame           string `json:"cpl_frame,omitempty"`
	Timecode           string `json:"timecode,omitempty"`
	OutputUnit         int64  `json:"output_unit"`
	FirstMissingSource int64  `json:"first_missing_source_frame"`
	OverageUnits       int64  `json:"overage_units"`
}

type PathReport struct {
	Path         string `json:"path"`
	ResolvedPath string `json:"resolved_path"`
	Exists       bool   `json:"exists"`
	ActualSize   int64  `json:"actual_size,omitempty"`
	ExpectedSize int64  `json:"expected_size,omitempty"`
	SizeOK       bool   `json:"size_ok"`
}

type ReferenceReport struct {
	InPKL      bool         `json:"in_pkl"`
	InAssetMap bool         `json:"in_assetmap"`
	Paths      []PathReport `json:"paths,omitempty"`
	Problems   []string     `json:"problems,omitempty"`
}

type AssetReport struct {
	ReelIndex              int              `json:"reel_index"`
	Kind                   string           `json:"kind"`
	ID                     string           `json:"id"`
	EditRate               string           `json:"edit_rate"`
	EntryPoint             int64            `json:"entry_point"`
	IntrinsicDuration      int64            `json:"intrinsic_duration"`
	Duration               int64            `json:"duration"`
	AvailableUnits         int64            `json:"available_units"`
	RequiredEnd            int64            `json:"required_end"`
	OverageUnits           int64            `json:"overage_units"`
	FirstMissingOutputUnit int64            `json:"first_missing_output_unit"`
	FirstMissingSource     int64            `json:"first_missing_source_frame"`
	ReelRelativeSeconds    string           `json:"reel_relative_seconds,omitempty"`
	AbsoluteSeconds        string           `json:"absolute_seconds,omitempty"`
	CPLFrame               string           `json:"cpl_frame,omitempty"`
	Timecode               string           `json:"timecode,omitempty"`
	Status                 string           `json:"status"`
	Problems               []string         `json:"problems,omitempty"`
	Reference              *ReferenceReport `json:"reference,omitempty"`
	momentSeconds          *big.Rat
}

type Report struct {
	CPLPath                string        `json:"cpl_path"`
	Root                   string        `json:"root"`
	CompositionEditRate    string        `json:"composition_edit_rate,omitempty"`
	TotalSeconds           string        `json:"total_seconds"`
	AssetCount             int           `json:"asset_count"`
	Assets                 []AssetReport `json:"assets"`
	FirstUnplayable        *Moment       `json:"first_unplayable,omitempty"`
	ReelDurationMismatches []string      `json:"reel_duration_mismatches,omitempty"`
	Errors                 []string      `json:"errors,omitempty"`
	Warnings               []string      `json:"warnings,omitempty"`
}

func Analyze(pkg *Package) *Report {
	report := &Report{
		CPLPath: pkg.CPLPath,
		Root:    pkg.Root,
		Assets:  []AssetReport{},
	}

	compositionRate, rateErr := parseRational(pkg.CPL.childText("EditRate"))
	if rateErr != nil {
		report.Errors = append(report.Errors, "composition EditRate: "+rateErr.Error())
	} else {
		report.CompositionEditRate = compositionRate.String()
	}

	reels := reelsOf(pkg.CPL)
	if len(reels) == 0 {
		report.Errors = append(report.Errors, "CPL contains no Reel elements")
	}

	reelStart := new(big.Rat)
	var bestMoment *Moment
	bestSeconds := new(big.Rat)

	for reelIndex, reel := range reels {
		assetNodes := assetsOf(&reel)
		var reelDuration *big.Rat
		var declaredDuration *big.Rat

		for _, node := range assetNodes {
			asset := analyzeAsset(pkg, &node, reelIndex, compositionRate, reelStart)
			report.Assets = append(report.Assets, asset)

			if asset.momentSeconds != nil {
				if bestMoment == nil || asset.momentSeconds.Cmp(bestSeconds) < 0 {
					secondsCopy := new(big.Rat).Set(asset.momentSeconds)
					bestSeconds = secondsCopy
					bestMoment = momentFromAsset(asset, secondsCopy, compositionRate)
				}
			}

			if rate, err := parseRational(asset.EditRate); err == nil && rate.valid() && asset.Duration >= 0 {
				duration := rate.unitsToSeconds(asset.Duration)
				if reelDuration == nil {
					reelDuration = duration
				} else if duration.Cmp(reelDuration) > 0 {
					reelDuration = duration
				}
				if declaredDuration == nil {
					declaredDuration = duration
				} else if duration.Cmp(declaredDuration) != 0 {
					message := "reel " + strconv.Itoa(reelIndex+1) + " has assets with unequal declared Duration/EditRate spans"
					report.ReelDurationMismatches = append(report.ReelDurationMismatches, message)
				}
			}
		}

		if reelDuration == nil {
			reelDuration = new(big.Rat)
		}
		reelStart.Add(reelStart, reelDuration)
	}

	report.AssetCount = len(report.Assets)
	report.TotalSeconds = formatRat(reelStart, 9)
	report.FirstUnplayable = bestMoment

	if pkg.PKLPath == "" {
		report.Warnings = append(report.Warnings, "PKL not found; PKL references were not checked")
	}
	if pkg.AssetMapPath == "" {
		report.Warnings = append(report.Warnings, "AssetMap not found; file paths were not checked")
	}
	return report
}

func reelsOf(cpl *xmlNode) []xmlNode {
	list := cpl.child("ReelList")
	if list != nil {
		if reels := list.children("Reel"); len(reels) > 0 {
			return reels
		}
	}
	return cpl.children("Reel")
}

func assetsOf(reel *xmlNode) []xmlNode {
	assetList := reel.child("AssetList")
	if assetList == nil {
		assetList = reel.child("Assets")
	}
	if assetList == nil {
		return nil
	}
	var assets []xmlNode
	for _, node := range assetList.Nodes {
		if node.child("IntrinsicDuration") != nil || childTextAny(&node, "Id", "ID", "TrackFileId") != "" {
			assets = append(assets, node)
		}
	}
	return assets
}
