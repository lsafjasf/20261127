package hdrframe

import (
	"fmt"
	"sort"
)

func findDuplicates(records []resolvedRecord, policy Policy) []Duplicate {
	groups := make(map[string][]resolvedRecord)
	order := make([]string, 0)
	for _, record := range records {
		key := recordKey(record)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], record)
	}
	sort.Strings(order)

	var out []Duplicate
	for _, key := range order {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		dup := Duplicate{Key: key, Resolution: policy.Duplicate}
		allIdentical := true
		firstFP := group[0].fingerprint
		for _, record := range group {
			dup.Versions = append(dup.Versions, record.version)
			dup.Checksums = append(dup.Checksums, record.fingerprint)
			if record.fingerprint != firstFP {
				allIdentical = false
			}
			if dup.Sequence == 0 {
				dup.Sequence = record.meta.sequenceValue()
			}
			if dup.Frame == nil {
				if frame, ok := record.meta.frameValue(); ok {
					frameCopy := frame
					dup.Frame = &frameCopy
				}
			}
		}
		dup.Identical = allIdentical
		chosen := chooseDuplicate(group, policy.Duplicate)
		if chosen != nil {
			dup.ChosenVersion = chosen.version
			if allIdentical {
				dup.Resolution = "identical_deduplicated"
			} else {
				dup.Resolution = "conflict_resolved_" + policy.Duplicate
			}
		} else {
			dup.Resolution = "ambiguous_" + policy.Duplicate
		}
		out = append(out, dup)
	}
	return out
}

func chooseDuplicate(group []resolvedRecord, mode string) *resolvedRecord {
	if len(group) == 0 {
		return nil
	}
	sorted := append([]resolvedRecord(nil), group...)
	sort.SliceStable(sorted, func(i, j int) bool {
		oi := orderValue(sorted[i])
		oj := orderValue(sorted[j])
		if oi != oj {
			return oi < oj
		}
		return sorted[i].index < sorted[j].index
	})
	if mode == "first" {
		return &sorted[0]
	}
	if mode == "error" {
		fp := sorted[0].fingerprint
		for _, record := range sorted[1:] {
			if record.fingerprint != fp {
				return nil
			}
		}
		return &sorted[len(sorted)-1]
	}
	return &sorted[len(sorted)-1]
}

func orderValue(record resolvedRecord) int {
	if record.meta.BitstreamOrder != 0 {
		return record.meta.BitstreamOrder
	}
	return record.meta.IngestOrder
}

func findSequenceGaps(records []resolvedRecord, policy Policy) []SequenceGap {
	bySeq := make(map[int64]resolvedRecord)
	var sequences []int64
	for _, record := range records {
		seq := record.meta.sequenceValue()
		if seq <= 0 {
			continue
		}
		if _, exists := bySeq[seq]; !exists {
			sequences = append(sequences, seq)
			bySeq[seq] = record
		}
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })

	var gaps []SequenceGap
	for i := 1; i < len(sequences); i++ {
		prev := sequences[i-1]
		next := sequences[i]
		if next <= prev+1 {
			continue
		}
		gap := SequenceGap{
			AfterSequence: prev, BeforeSequence: next,
			MissingCount: next - prev - 1,
			Action:       policy.SequenceGap,
		}
		if prevRecord := bySeq[prev]; prevRecord.meta.Frame != nil {
			frame := *prevRecord.meta.Frame
			gap.AfterFrame = &frame
		}
		if nextRecord := bySeq[next]; nextRecord.meta.Frame != nil {
			frame := *nextRecord.meta.Frame
			gap.BeforeFrame = &frame
		}
		limit := gap.MissingCount
		if limit > 100 {
			limit = 100
		}
		for seq := prev + 1; seq < next && int64(len(gap.Missing)) < limit; seq++ {
			gap.Missing = append(gap.Missing, seq)
		}
		gaps = append(gaps, gap)
	}
	return gaps
}

func findSceneChanges(records []resolvedRecord) []SceneChange {
	sorted := append([]resolvedRecord(nil), records...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].effective == sorted[j].effective {
			return sorted[i].index < sorted[j].index
		}
		return sorted[i].effective < sorted[j].effective
	})
	var changes []SceneChange
	var lastScene *int64
	for _, record := range sorted {
		scene := record.meta.sceneValue()
		if scene == nil {
			continue
		}
		if lastScene != nil && *lastScene != *scene {
			frame, _ := record.meta.frameValue()
			changes = append(changes, SceneChange{
				Frame: frame, FromScene: lastScene, ToScene: scene,
			})
		}
		lastScene = scene
	}
	return changes
}

func int64PtrString(p *int64) string {
	if p == nil {
		return "<none>"
	}
	return fmt.Sprintf("%d", *p)
}
