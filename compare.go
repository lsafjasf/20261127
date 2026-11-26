package colorcompare

import (
	"fmt"
	"sort"
)

// PairComparison contains independent distance measures and the exact
// CIEDE2000 component terms for one pair of RGB24 samples.
type PairComparison struct {
	A              RGB24
	B              RGB24
	LabA           Lab
	LabB           Lab
	EuclideanRGB   float64
	EuclideanLab   float64
	DeltaE2000     float64
	Components     DeltaEComponents
	Dominant       Dimension
	EuclideanEqual bool
	DeltaEEqual    bool
}

// CompareRGB computes all supported measures for one sample pair. No metric is
// multiplied, normalized or clamped; each value remains in its native units.
func CompareRGB(a, b RGB24) PairComparison {
	labA := LabFromRGB(a)
	labB := LabFromRGB(b)
	deltaE, components := DeltaE2000Details(labA, labB)
	return PairComparison{
		A:              a,
		B:              b,
		LabA:           labA,
		LabB:           labB,
		EuclideanRGB:   EuclideanRGB(a, b),
		EuclideanLab:   EuclideanLab(labA, labB),
		DeltaE2000:     deltaE,
		Components:     components,
		Dominant:       components.Dominant(),
		EuclideanEqual: a == b,
		DeltaEEqual:    deltaE == 0,
	}
}

// Sample is one named pair of RGB24 outputs. A and B are independent samples
// from the two converters being compared.
type Sample struct {
	ID string
	A  RGB24
	B  RGB24
}

// SampleResult is a PairComparison plus ranks among all analyzed samples.
type SampleResult struct {
	ID string
	PairComparison
	EuclideanRank  int
	DeltaE2000Rank int
}

// RankReversal records a pair of samples whose order differs between RGB
// Euclidean distance and CIEDE2000.
type RankReversal struct {
	SampleA          string
	SampleB          string
	EuclideanCloser  string
	DeltaE2000Closer string
}

// Analysis compares samples individually and also compares their ordering by
// each metric. Ranks are ascending: rank 1 is the smallest difference.
type Analysis struct {
	Samples         []SampleResult
	EuclideanOrder  []string
	DeltaE2000Order []string
	Reversals       []RankReversal
}

// AnalyzeSamples validates IDs, computes every sample, and checks all sample
// pairs for ranking reversals. It does not transform any distance value.
func AnalyzeSamples(samples []Sample) (Analysis, error) {
	if len(samples) == 0 {
		return Analysis{}, fmt.Errorf("at least one sample is required")
	}

	seen := make(map[string]struct{}, len(samples))
	results := make([]SampleResult, 0, len(samples))
	for i, sample := range samples {
		if sample.ID == "" {
			return Analysis{}, fmt.Errorf("sample %d has an empty ID", i+1)
		}
		if _, exists := seen[sample.ID]; exists {
			return Analysis{}, fmt.Errorf("duplicate sample ID %q", sample.ID)
		}
		seen[sample.ID] = struct{}{}
		results = append(results, SampleResult{
			ID:             sample.ID,
			PairComparison: CompareRGB(sample.A, sample.B),
		})
	}

	euclideanOrder := metricOrder(results, func(result SampleResult) float64 {
		return result.EuclideanRGB
	})
	deltaEOrder := metricOrder(results, func(result SampleResult) float64 {
		return result.DeltaE2000
	})
	assignRanks(results, euclideanOrder, func(result *SampleResult, rank int) {
		result.EuclideanRank = rank
	})
	assignRanks(results, deltaEOrder, func(result *SampleResult, rank int) {
		result.DeltaE2000Rank = rank
	})

	names := make([]string, len(euclideanOrder))
	for i, index := range euclideanOrder {
		names[i] = results[index].ID
	}
	deltaENames := make([]string, len(deltaEOrder))
	for i, index := range deltaEOrder {
		deltaENames[i] = results[index].ID
	}

	return Analysis{
		Samples:         results,
		EuclideanOrder:  names,
		DeltaE2000Order: deltaENames,
		Reversals:       findReversals(results),
	}, nil
}

func metricOrder(results []SampleResult, metric func(SampleResult) float64) []int {
	order := make([]int, len(results))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		left := results[order[i]]
		right := results[order[j]]
		lv, rv := metric(left), metric(right)
		if lv == rv {
			return left.ID < right.ID
		}
		return lv < rv
	})
	return order
}

func assignRanks(results []SampleResult, order []int, set func(*SampleResult, int)) {
	for rank, index := range order {
		set(&results[index], rank+1)
	}
}

func findReversals(results []SampleResult) []RankReversal {
	var reversals []RankReversal
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			left, right := results[i], results[j]
			euclideanOrder := compareDistances(left.EuclideanRGB, right.EuclideanRGB)
			deltaEOrder := compareDistances(left.DeltaE2000, right.DeltaE2000)
			if euclideanOrder == 0 || deltaEOrder == 0 || euclideanOrder == deltaEOrder {
				continue
			}
			euclideanCloser, deltaECloser := left.ID, left.ID
			if euclideanOrder > 0 {
				euclideanCloser = right.ID
			}
			if deltaEOrder > 0 {
				deltaECloser = right.ID
			}
			reversals = append(reversals, RankReversal{
				SampleA:          left.ID,
				SampleB:          right.ID,
				EuclideanCloser:  euclideanCloser,
				DeltaE2000Closer: deltaECloser,
			})
		}
	}
	return reversals
}

func compareDistances(left, right float64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
