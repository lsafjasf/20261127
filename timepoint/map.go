package timepoint

import (
	"fmt"
	"sort"
)

// Cut removes source interval [StartNS, EndNS). The start and end boundary
// resolve to the same target junction; interior points cannot be migrated.
type Cut struct {
	StartNS uint64 `json:"start_ns"`
	EndNS   uint64 `json:"end_ns"`
	Reason  string `json:"reason,omitempty"`
}

// TimelineMap maps old timeline points to a cut/re-timed target.
type TimelineMap struct {
	OffsetNS   int64   `json:"offset_ns,omitempty"`
	Cuts       []Cut   `json:"cuts,omitempty"`
	DurationNS *uint64 `json:"duration_ns,omitempty"`
}

func (tm TimelineMap) Normalized() (TimelineMap, error) {
	out := TimelineMap{OffsetNS: tm.OffsetNS, DurationNS: tm.DurationNS}
	out.Cuts = append(out.Cuts, tm.Cuts...)
	sort.Slice(out.Cuts, func(i, j int) bool { return out.Cuts[i].StartNS < out.Cuts[j].StartNS })
	for i, cut := range out.Cuts {
		if cut.EndNS < cut.StartNS {
			return TimelineMap{}, fmt.Errorf("cut %d ends before it starts", i)
		}
		if i > 0 && cut.StartNS < out.Cuts[i-1].EndNS {
			return TimelineMap{}, fmt.Errorf("cuts %d and %d overlap", i-1, i)
		}
	}
	return out, nil
}

func (tm TimelineMap) MapPoint(p Point) Point {
	if !p.Ok() {
		return p
	}
	mapped, status, reason := tm.MapNS(*p.NS)
	if status != StatusResolved {
		return Unresolved(p.Raw, status, reason)
	}
	return Resolved(mapped)
}

func (tm TimelineMap) MapNS(source uint64) (uint64, Status, string) {
	normalized, err := tm.Normalized()
	if err != nil {
		return 0, StatusTimelineConflict, err.Error()
	}
	var removedBefore uint64
	for _, cut := range normalized.Cuts {
		if source > cut.StartNS && source < cut.EndNS {
			reason := cut.Reason
			if reason == "" {
				reason = fmt.Sprintf("source point %s lies in removed interval [%s,%s)",
					FormatNS(source), FormatNS(cut.StartNS), FormatNS(cut.EndNS))
			}
			return 0, StatusRemoved, reason
		}
		if source >= cut.EndNS {
			removedBefore += cut.EndNS - cut.StartNS
		}
	}

	signed := int64(source) - int64(removedBefore) + normalized.OffsetNS
	if signed < 0 {
		return 0, StatusBeforeZero, fmt.Sprintf("mapping gives negative time %d ns", signed)
	}
	target := uint64(signed)
	if normalized.DurationNS != nil && target > *normalized.DurationNS {
		return 0, StatusOutOfTargetRange,
			fmt.Sprintf("mapped time %s exceeds target duration %s",
				FormatNS(target), FormatNS(*normalized.DurationNS))
	}
	return target, StatusResolved, ""
}
