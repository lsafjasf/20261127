package multicam

import "sort"

const intervalEpsilon = 1e-9

func projectInterval(local Interval, offset, masterDuration float64) (Interval, Interval, bool) {
	master := Interval{
		StartSeconds: local.StartSeconds + offset,
		EndSeconds:   local.EndSeconds + offset,
	}

	start := master.StartSeconds
	if start < 0 {
		start = 0
	}
	end := master.EndSeconds
	if end > masterDuration {
		end = masterDuration
	}
	if end <= start {
		return master, Interval{}, false
	}
	return master, Interval{StartSeconds: start, EndSeconds: end}, true
}

// mergeIntervals merges overlaps and true adjacency, but keeps real recording
// gaps separate. That distinction is what preserves a one-minute downtime.
func mergeIntervals(intervals []Interval) []Interval {
	if len(intervals) == 0 {
		return []Interval{}
	}

	sorted := append([]Interval(nil), intervals...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].StartSeconds == sorted[j].StartSeconds {
			return sorted[i].EndSeconds < sorted[j].EndSeconds
		}
		return sorted[i].StartSeconds < sorted[j].StartSeconds
	})

	merged := make([]Interval, 0, len(sorted))
	current := sorted[0]
	for _, next := range sorted[1:] {
		if next.StartSeconds <= current.EndSeconds+intervalEpsilon {
			if next.EndSeconds > current.EndSeconds {
				current.EndSeconds = next.EndSeconds
			}
			continue
		}
		merged = append(merged, current)
		current = next
	}
	return append(merged, current)
}

func coveredSeconds(intervals []Interval) float64 {
	var total float64
	for _, interval := range intervals {
		total += interval.EndSeconds - interval.StartSeconds
	}
	return total
}
