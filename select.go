package hdrframe

import "sort"

func selectResolved(records []resolvedRecord, atSeconds, tolerance float64, policy Policy) (*resolvedRecord, bool) {
	eligible := make([]resolvedRecord, 0)
	for _, record := range records {
		if record.effective <= atSeconds+tolerance {
			eligible = append(eligible, record)
		}
	}
	if len(eligible) == 0 {
		return nil, false
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := eligible[i], eligible[j]
		if !almostEqual(a.effective, b.effective, tolerance) {
			return a.effective < b.effective
		}
		sa, sb := a.meta.sequenceValue(), b.meta.sequenceValue()
		if sa != sb {
			return sa < sb
		}
		oa, ob := orderValue(a), orderValue(b)
		if oa != ob {
			return oa < ob
		}
		return a.index < b.index
	})

	last := eligible[len(eligible)-1]
	group := []resolvedRecord{last}
	for i := len(eligible) - 2; i >= 0; i-- {
		if almostEqual(eligible[i].effective, last.effective, tolerance) {
			group = append(group, eligible[i])
		}
	}
	chosen := chooseDuplicate(group, policy.Duplicate)
	if chosen == nil {
		return &last, true
	}
	return chosen, false
}
