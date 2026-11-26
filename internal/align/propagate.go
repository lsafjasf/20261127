package align

import (
	"fmt"
	"math"
	"sort"
)

type prediction struct {
	source     *segmentState
	local      float64
	master     float64
	confidence float64
}

func propagateEvents(idx *indexedSession, states []*segmentState, diag *diagnosticSet) bool {
	changed := false
	byEvent := make(map[string][]prediction)
	for _, state := range states {
		if state.status != "locked" || state.fit == nil {
			continue
		}
		for _, obs := range state.segment.Observations {
			event := idx.events[obs.Event]
			if event.master != nil {
				continue
			}
			confidence := observationConfidence(obs) * fitPropagationConfidence(state.fit, idx.config)
			if confidence < idx.config.MinAnchorConfidence {
				continue
			}
			byEvent[obs.Event] = append(byEvent[obs.Event], prediction{
				source: state, local: obs.Local,
				master: state.fit.mapTime(obs.Local), confidence: confidence,
			})
		}
	}

	for eventID, predictions := range byEvent {
		event := idx.events[eventID]
		if event.master != nil || len(predictions) == 0 {
			continue
		}
		sort.Slice(predictions, func(i, j int) bool {
			return predictions[i].master < predictions[j].master
		})
		window := idx.config.AnchorToleranceMS / 1000 * 1.5
		best := make([]prediction, 0)
		second := make([]prediction, 0)
		for i := range predictions {
			candidate := make([]prediction, 0)
			for j := i; j < len(predictions); j++ {
				if predictions[j].master-predictions[i].master > window {
					break
				}
				candidate = append(candidate, predictions[j])
			}
			if clusterScore(candidate) > clusterScore(best) {
				second = best
				best = candidate
			} else if clusterScore(candidate) > clusterScore(second) {
				second = candidate
			}
		}
		if len(best) == 0 {
			continue
		}
		if len(second) > 0 && clusterScore(second) >= clusterScore(best)*0.8 {
			diag.add(Diagnostic{
				Severity: "warning", Code: "conflicting_event_predictions", Event: eventID,
				Message: "camera predictions disagree; master time left unresolved",
			})
			continue
		}
		weighted, confidence, devices := weightedPrediction(best)
		if confidence < idx.config.MinAnchorConfidence {
			continue
		}
		value := weighted
		event.master = &value
		event.source = "propagated"
		event.confidence = math.Min(1, confidence*idx.config.PropagationFactor)
		diag.add(Diagnostic{
			Severity: "info", Code: "event_time_propagated", Event: eventID,
			Message: fmt.Sprintf("derived from %d prediction(s) on %d locked segment(s)", len(best), devices),
		})
		changed = true
	}
	return changed
}

func fitPropagationConfidence(fit *fitResult, cfg Config) float64 {
	base := 0.70 + 0.08*float64(minInt(len(fit.inliers), 5))
	if fit.maxMS > 0 {
		base -= 0.10 * math.Min(1, fit.maxMS/cfg.AnchorToleranceMS)
	}
	if !fit.driftConfirmed {
		base *= 0.85
	}
	return math.Min(1, math.Max(0, base))
}

func clusterScore(points []prediction) float64 {
	if len(points) == 0 {
		return 0
	}
	var total float64
	segments := make(map[string]bool)
	for _, point := range points {
		total += point.confidence
		segments[point.source.segment.ID] = true
	}
	return total + float64(len(segments))*0.15
}

func weightedPrediction(points []prediction) (float64, float64, int) {
	var weighted, weights float64
	segments := make(map[string]bool)
	for _, point := range points {
		weighted += point.master * point.confidence
		weights += point.confidence
		segments[point.source.segment.ID] = true
	}
	if weights == 0 {
		return 0, 0, 0
	}
	return weighted / weights, weights / float64(len(points)), len(segments)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
