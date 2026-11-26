package gamutcompare

import (
	"math"
	"sort"
)

const angleMergeEps = 1e-9

func unionCircularIntervals(raw []angularInterval) []angularInterval {
	twoPi := 2 * math.Pi
	var linear []angularInterval
	for _, interval := range raw {
		lo := normAngle(interval.lo)
		width := interval.hi - interval.lo
		hi := lo + width
		if hi <= twoPi+angleMergeEps {
			linear = append(linear, angularInterval{lo: lo, hi: math.Min(hi, twoPi)})
		} else {
			linear = append(linear, angularInterval{lo: lo, hi: twoPi})
			linear = append(linear, angularInterval{lo: 0, hi: normAngle(hi)})
		}
	}
	if len(linear) == 0 {
		return nil
	}

	sort.Slice(linear, func(i, j int) bool { return linear[i].lo < linear[j].lo })
	merged := []angularInterval{linear[0]}
	for _, interval := range linear[1:] {
		last := &merged[len(merged)-1]
		if interval.lo <= last.hi+angleMergeEps {
			last.hi = math.Max(last.hi, interval.hi)
			continue
		}
		merged = append(merged, interval)
	}

	if len(merged) == 1 && merged[0].lo <= angleMergeEps && twoPi-merged[0].hi <= angleMergeEps {
		return []angularInterval{{lo: 0, hi: twoPi}}
	}

	if len(merged) >= 2 && merged[0].lo <= angleMergeEps && twoPi-merged[len(merged)-1].hi <= angleMergeEps {
		first := merged[0]
		last := merged[len(merged)-1]
		wrapped := angularInterval{lo: last.lo, hi: first.hi + twoPi}
		result := append([]angularInterval{}, merged[1:len(merged)-1]...)
		result = append(result, wrapped)
		sort.Slice(result, func(i, j int) bool { return result[i].lo < result[j].lo })
		return result
	}
	return merged
}
