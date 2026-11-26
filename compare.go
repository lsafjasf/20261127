package gamutcompare

import (
	"math"
	"strings"
)

const nearlyEqualAreaTolerance = 0.01

func compareDisplays(reports []DisplayReport) Comparison {
	comparison := Comparison{Pairs: make([]PairComparison, 0)}
	if len(reports) == 0 {
		return comparison
	}

	maxArea := reports[0].Area.XY
	maxUpVP := reports[0].Area.UpVP
	maxCoverage := reports[0].Coverage.TargetCoverage
	for _, candidate := range reports[1:] {
		maxArea = math.Max(maxArea, candidate.Area.XY)
		maxUpVP = math.Max(maxUpVP, candidate.Area.UpVP)
		maxCoverage = math.Max(maxCoverage, candidate.Coverage.TargetCoverage)
	}

	const comparisonEps = 1e-10
	for _, candidate := range reports {
		if math.Abs(candidate.Area.XY-maxArea) <= comparisonEps {
			comparison.AreaLeaders = append(comparison.AreaLeaders, candidate.Name)
		}
		if math.Abs(candidate.Area.UpVP-maxUpVP) <= comparisonEps {
			comparison.UpVPAreaLeaders = append(comparison.UpVPAreaLeaders, candidate.Name)
		}
		if math.Abs(candidate.Coverage.TargetCoverage-maxCoverage) <= comparisonEps {
			comparison.TargetCoverageLeaders = append(comparison.TargetCoverageLeaders, candidate.Name)
		}
	}
	comparison.LargestAreaDisplay = leaderLabel(comparison.AreaLeaders)
	comparison.LargestUpVPAreaDisplay = leaderLabel(comparison.UpVPAreaLeaders)
	comparison.HighestTargetCoverageDisplay = leaderLabel(comparison.TargetCoverageLeaders)
	comparison.AreaLeaderIsCoverageLeader = len(comparison.AreaLeaders) == 1 &&
		len(comparison.TargetCoverageLeaders) == 1 &&
		comparison.AreaLeaders[0] == comparison.TargetCoverageLeaders[0]

	for i := 0; i < len(reports); i++ {
		for j := i + 1; j < len(reports); j++ {
			comparison.Pairs = append(comparison.Pairs, pairCompare(reports[i], reports[j]))
		}
	}
	return comparison
}

func leaderLabel(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return "tie: " + strings.Join(names, ", ")
}

func pairCompare(a, b DisplayReport) PairComparison {
	areaDiff := a.Area.XY - b.Area.XY
	coverageDiff := (a.Coverage.TargetCoverage - b.Coverage.TargetCoverage) * 100

	pair := PairComparison{
		A:                            a.Name,
		B:                            b.Name,
		AreaDifferenceXY:             areaDiff,
		AreaRatio:                    ratio(a.Area.XY, b.Area.XY),
		AreaRelativeDifference:       math.Abs(areaDiff) / math.Max(math.Abs(a.Area.XY), math.Abs(b.Area.XY)),
		CoverageDifferencePercentPts: coverageDiff,
		NearlyEqualArea:              math.Abs(areaDiff)/math.Max(a.Area.XY, b.Area.XY) <= nearlyEqualAreaTolerance,
	}

	switch {
	case areaDiff > geometryEps:
		pair.LargerArea = a.Name
	case areaDiff < -geometryEps:
		pair.LargerArea = b.Name
	default:
		pair.LargerArea = "equal"
	}

	switch {
	case coverageDiff > 1e-10:
		pair.HigherTargetCoverage = a.Name
	case coverageDiff < -1e-10:
		pair.HigherTargetCoverage = b.Name
	default:
		pair.HigherTargetCoverage = "equal"
	}

	pair.AreaAndCoverageOrdersDiffer = pair.LargerArea != "equal" &&
		pair.HigherTargetCoverage != "equal" &&
		pair.LargerArea != pair.HigherTargetCoverage
	return pair
}
