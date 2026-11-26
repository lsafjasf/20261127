package check

import (
	"fmt"
	"sort"
)

type indexedObservation struct {
	index int
	value Observation
}

func Analyze(bundle Bundle) (*Report, error) {
	if err := validate(bundle); err != nil {
		return nil, err
	}

	byFrame := make(map[string][]indexedObservation)
	frameRefs := make(map[string]FrameRef)
	recovery := make(map[string]Recovery)

	for i, obs := range bundle.Observations {
		key := obs.Frame.Key
		byFrame[key] = append(byFrame[key], indexedObservation{index: i, value: obs})
		frameRefs[key] = mergeFrameRef(frameRefs[key], obs.Frame)
	}
	for _, rec := range bundle.Recovery {
		recovery[rec.FrameKey] = rec
		if _, ok := frameRefs[rec.FrameKey]; !ok {
			frameRefs[rec.FrameKey] = FrameRef{Key: rec.FrameKey}
		}
	}

	keys := make([]string, 0, len(frameRefs))
	for key := range frameRefs {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return frameLess(frameRefs[keys[i]], frameRefs[keys[j]], keys[i], keys[j])
	})

	report := &Report{FrameCount: len(keys)}
	children, referenceGraphProvided := referenceGraph(frameRefs, keys)
	hardFailures := make(map[string]bool)
	for _, obs := range bundle.Observations {
		if obs.Status == StatusFail {
			hardFailures[obs.Frame.Key] = true
		}
	}
	impactedBy := failureImpacts(hardFailures, children)
	for _, key := range keys {
		items := byFrame[key]
		sort.SliceStable(items, func(i, j int) bool {
			left, right := items[i].value, items[j].value
			if layerRank(left.Layer) != layerRank(right.Layer) {
				return layerRank(left.Layer) < layerRank(right.Layer)
			}
			return items[i].index < items[j].index
		})

		var recoveryPtr *Recovery
		if rec, ok := recovery[key]; ok {
			recoveryCopy := rec
			recoveryPtr = &recoveryCopy
		}
		verdict := buildVerdict(frameRefs[key], items, recoveryPtr, impactedBy[key], referenceGraphProvided)
		report.FrameVerdicts = append(report.FrameVerdicts, verdict)
		if report.EarliestByLayer == nil {
			report.EarliestByLayer = make(map[Layer]Earliest)
		}

		for _, item := range items {
			obs := item.value
			if obs.Status != StatusFail {
				continue
			}
			frame := frameRefs[key]
			candidate := makeEarliest(obs)
			if report.Earliest == nil || earliestLess(candidate, *report.Earliest, frame) {
				copyObs := obs
				copyObs.Frame = frame
				candidate.Observation = &copyObs
				report.Earliest = &candidate
			}
			if report.EarliestDisplay == nil || displayLess(candidate, *report.EarliestDisplay, frame) {
				copyObs := obs
				copyObs.Frame = frame
				candidate.Observation = &copyObs
				report.EarliestDisplay = &candidate
			}
			current, exists := report.EarliestByLayer[obs.Layer]
			if !exists || earliestLess(candidate, current, frame) {
				layerCandidate := candidate
				copyObs := obs
				copyObs.Frame = frame
				layerCandidate.Observation = &copyObs
				report.EarliestByLayer[obs.Layer] = layerCandidate
			}
		}
	}

	report.GlobalWarnings = globalWarnings(bundle.Observations)
	return report, nil
}

func validate(bundle Bundle) error {
	for i, obs := range bundle.Observations {
		if obs.Frame.Key == "" {
			return fmt.Errorf("observations[%d].frame.key is required", i)
		}
		if !validLayer(obs.Layer) {
			return fmt.Errorf("observations[%d].layer %q is invalid", i, obs.Layer)
		}
		if !validStatus(obs.Status) {
			return fmt.Errorf("observations[%d].status %q is invalid", i, obs.Status)
		}
	}
	for i, rec := range bundle.Recovery {
		if rec.FrameKey == "" {
			return fmt.Errorf("recovery[%d].frame_key is required", i)
		}
	}
	return nil
}

func validLayer(layer Layer) bool {
	switch layer {
	case LayerContainer, LayerBitstream, LayerDecode, LayerPixels:
		return true
	default:
		return false
	}
}

func validStatus(status Status) bool {
	switch status {
	case StatusPass, StatusFail, StatusWarn, StatusUnknown:
		return true
	default:
		return false
	}
}

func layerRank(layer Layer) int {
	switch layer {
	case LayerContainer:
		return 0
	case LayerBitstream:
		return 1
	case LayerDecode:
		return 2
	case LayerPixels:
		return 3
	default:
		return 99
	}
}

func frameLess(a FrameRef, b FrameRef, keyA, keyB string) bool {
	if a.DecodeOrder != b.DecodeOrder {
		return a.DecodeOrder < b.DecodeOrder
	}
	return keyA < keyB
}

func mergeFrameRef(current FrameRef, next FrameRef) FrameRef {
	merged := current
	merged.Key = next.Key
	if next.DecodeOrder != 0 {
		merged.DecodeOrder = next.DecodeOrder
	}
	if next.DisplayOrder != nil {
		value := *next.DisplayOrder
		merged.DisplayOrder = &value
	}
	if next.PTS != nil {
		value := *next.PTS
		merged.PTS = &value
	}
	if next.DTS != nil {
		value := *next.DTS
		merged.DTS = &value
	}
	if next.KeyFrame != nil {
		value := *next.KeyFrame
		merged.KeyFrame = &value
	}
	merged.References = mergeStrings(merged.References, next.References)
	return merged
}

func mergeStrings(current, next []string) []string {
	seen := make(map[string]bool, len(current)+len(next))
	merged := make([]string, 0, len(current)+len(next))
	for _, value := range append(append([]string{}, current...), next...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		merged = append(merged, value)
	}
	return merged
}

func referenceGraph(frames map[string]FrameRef, keys []string) (map[string][]string, bool) {
	children := make(map[string][]string)
	provided := false
	for _, key := range keys {
		for _, reference := range frames[key].References {
			provided = true
			children[reference] = append(children[reference], key)
		}
	}
	return children, provided
}

func failureImpacts(failures map[string]bool, children map[string][]string) map[string][]string {
	impacts := make(map[string][]string)
	for failedKey := range failures {
		visited := map[string]bool{failedKey: true}
		queue := append([]string{}, children[failedKey]...)
		for len(queue) > 0 {
			key := queue[0]
			queue = queue[1:]
			if visited[key] {
				continue
			}
			visited[key] = true
			impacts[key] = appendUniqueString(impacts[key], failedKey)
			queue = append(queue, children[key]...)
		}
	}
	for key := range impacts {
		sort.Strings(impacts[key])
	}
	return impacts
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func makeEarliest(obs Observation) Earliest {
	return Earliest{
		DecodeOrder: obs.Frame.DecodeOrder,
		FrameKey:    obs.Frame.Key,
		Layer:       obs.Layer,
		Check:       obs.Check,
		Status:      obs.Status,
	}
}

func earliestLess(a Earliest, b Earliest, frame FrameRef) bool {
	if a.DecodeOrder != b.DecodeOrder {
		return a.DecodeOrder < b.DecodeOrder
	}
	return layerRank(a.Layer) < layerRank(b.Layer)
}

func displayOrder(frame FrameRef) int64 {
	if frame.DisplayOrder != nil {
		return int64(*frame.DisplayOrder)
	}
	if frame.PTS != nil {
		return *frame.PTS
	}
	return int64(frame.DecodeOrder)
}

func displayLess(a Earliest, b Earliest, frame FrameRef) bool {
	orderA := displayOrder(frame)
	orderB := int64(b.DecodeOrder)
	if b.Observation != nil {
		orderB = displayOrder(b.Observation.Frame)
	}
	if orderA != orderB {
		return orderA < orderB
	}
	return layerRank(a.Layer) < layerRank(b.Layer)
}

func globalWarnings(observations []Observation) []string {
	var warnings []string
	crcPass := false
	crcPassPixelFail := false
	for _, obs := range observations {
		if obs.Layer == LayerBitstream && obs.Status == StatusPass {
			crcPass = true
		}
		if obs.Layer == LayerBitstream && obs.Status == StatusPass {
			for _, other := range observations {
				if other.Frame.Key == obs.Frame.Key && other.Layer == LayerPixels && other.Status == StatusFail {
					crcPassPixelFail = true
				}
			}
		}
	}
	if crcPass {
		warnings = append(warnings, "CRC 通过只证明被覆盖字节与存储/期望值一致，不能证明解码像素正确。")
	}
	if crcPassPixelFail {
		warnings = append(warnings, "出现 CRC 通过但像素校验失败：检查 CRC 覆盖范围、陈旧期望值、校验字段损坏、字节归一化、解码器确定性和哈希碰撞。")
	}
	return warnings
}
