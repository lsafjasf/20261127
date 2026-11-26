package iccproof

import "fmt"

// IntentComparison summarizes per-sample CIEDE2000 differences between the
// soft-proof outputs produced by two rendering intents.
type IntentComparison struct {
	IntentA Intent      `json:"intent_a"`
	IntentB Intent      `json:"intent_b"`
	DeltaE  DeltaEStats `json:"delta_e"`
}

func compareIntents(intents []Intent, results []SampleResult) []IntentComparison {
	bySample := map[string]map[Intent]SampleResult{}
	for _, result := range results {
		key := fmt.Sprintf("%d|%s|%d|%d", result.SampleIndex, result.SampleID, coordValue(result.X), coordValue(result.Y))
		if bySample[key] == nil {
			bySample[key] = map[Intent]SampleResult{}
		}
		bySample[key][result.Intent] = result
	}

	var comparisons []IntentComparison
	for i := 0; i < len(intents); i++ {
		for j := i + 1; j < len(intents); j++ {
			a, b := intents[i], intents[j]
			stats := DeltaEStats{}
			maxDE := -1.0
			var values []float64
			for _, pair := range bySample {
				ra, oka := pair[a]
				rb, okb := pair[b]
				if !oka || !okb || ra.MediaLab == nil || rb.MediaLab == nil {
					continue
				}
				if (ra.DeltaEReliable != nil && !*ra.DeltaEReliable) ||
					(rb.DeltaEReliable != nil && !*rb.DeltaEReliable) {
					continue
				}
				de := DeltaE00(*ra.MediaLab, *rb.MediaLab)
				values = append(values, de)
				if de > maxDE {
					maxDE = de
					stats.MaxSampleIndex = ra.SampleIndex
				}
			}
			if len(values) > 0 {
				stats.Count = len(values)
				stats.Min = minFloat(values)
				stats.Max = maxDE
				stats.Mean = mean(values)
			}
			comparisons = append(comparisons, IntentComparison{IntentA: a, IntentB: b, DeltaE: stats})
		}
	}
	return comparisons
}

func coordValue(v *int) int {
	if v == nil {
		return -1
	}
	return *v
}

func minFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
