package tearcheck

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const schemaVersion = "tearcheck/v1"

// AnalyzeObservations classifies precomputed observations and merges metadata.
// Low-confidence candidates are retained as manual-review findings.
func AnalyzeObservations(observations []Observation, meta Metadata) *Report {
	meta.CodecEvents = dedupeCodecEvents(meta.CodecEvents)
	merged := make([]Observation, 0, len(observations)+len(meta.CodecEvents)+len(meta.ContentEffectWindows))
	merged = append(merged, observations...)

	timingFaults := timingFaultByFrame(meta.FrameTiming)
	for i := range merged {
		applyMetadataToObservation(&merged[i], meta, timingFaults)
	}
	merged = appendMetadataOnlyObservations(merged, meta, timingFaults)

	report := &Report{
		SchemaVersion: schemaVersion,
		Source:        meta.Source,
		Width:         meta.Width,
		Height:        meta.Height,
		Codec:         meta.Codec,
		GeneratedAt:   time.Now().UTC(),
		Findings:      []Finding{},
		ManualReviews: []Finding{},
		Summary: Summary{
			ByCause:        map[Cause]int{},
			FramesAnalyzed: len(meta.FrameTiming),
		},
	}

	for _, observation := range merged {
		if !IsCandidate(observation) {
			continue
		}
		finding := ClassifyObservation(observation)
		finding.ID = fmt.Sprintf("F%04d-%03d", observation.FrameIndex, maxInt(observation.SeamY, 0))
		report.Findings = append(report.Findings, finding)
		report.Summary.Findings++
		report.Summary.ByCause[finding.Cause]++
		if finding.ReviewRequired {
			report.ManualReviews = append(report.ManualReviews, finding)
			report.Summary.ManualReview++
		}
	}

	sortFindings(report.Findings)
	sortFindings(report.ManualReviews)
	report.Summary.Overall = overallCause(report.Findings)
	return report
}

func dedupeCodecEvents(events []CodecEvent) []CodecEvent {
	type key string
	indexByKey := make(map[key]int)
	out := make([]CodecEvent, 0, len(events))
	for _, event := range events {
		id := key(fmt.Sprintf("f:%d:t:%.3f:y:%d:m:%s",
			event.FrameIndex, event.Time, event.Y, event.Message))
		if event.Y > 0 {
			id = key(fmt.Sprintf("f:%d:t:%.3f:y:%d", event.FrameIndex, event.Time, event.Y))
		}
		if index, ok := indexByKey[id]; ok {
			if event.Confidence > out[index].Confidence {
				out[index].Confidence = event.Confidence
			}
			continue
		}
		indexByKey[id] = len(out)
		out = append(out, event)
	}
	return out
}

func overallCause(findings []Finding) Cause {
	actionable := make(map[Cause]struct{}, 3)
	manual := 0
	for _, finding := range findings {
		if finding.ReviewRequired {
			manual++
			continue
		}
		switch finding.Cause {
		case CauseHardwareSync, CauseEncoding, CauseContent:
			actionable[finding.Cause] = struct{}{}
		}
	}
	if len(actionable) >= 2 {
		return CauseMixed
	}
	for cause := range actionable {
		return cause
	}
	if manual > 0 {
		return CauseManual
	}
	return CauseNormal
}

func applyMetadataToObservation(o *Observation, meta Metadata, timingFaults map[int]float64) {
	if fault, ok := timingFaults[o.FrameIndex]; ok {
		o.TimingFault = math.Max(o.TimingFault, fault)
	}
	for _, window := range meta.ContentEffectWindows {
		if observationInWindow(*o, window) {
			o.ContentMarker = math.Max(o.ContentMarker, Clamp01(window.Confidence))
			if window.Label != "" {
				o.Messages = appendUnique(o.Messages, "content window: "+window.Label)
			} else {
				o.Messages = appendUnique(o.Messages, "trusted content-effect window")
			}
		}
	}
	for _, event := range meta.CodecEvents {
		if !codecEventMatches(*o, event) {
			continue
		}
		o.CodecError = math.Max(o.CodecError, Clamp01(event.Confidence))
		if event.Y > 0 && o.SeamY > 0 && absInt(event.Y-o.SeamY) <= 8 {
			o.BlockAlignment = math.Max(o.BlockAlignment, 0.9)
		} else if event.Y > 0 && o.SeamY == 0 {
			o.SeamY = event.Y
			o.BlockAlignment = math.Max(o.BlockAlignment, 0.9)
		}
		if event.Message != "" {
			o.Messages = appendUnique(o.Messages, "decoder: "+event.Message)
		}
	}
}

func appendMetadataOnlyObservations(existing []Observation, meta Metadata, timingFaults map[int]float64) []Observation {
	out := append([]Observation{}, existing...)
	hasKey := func(frame, y int, kind string) bool {
		for _, o := range existing {
			if o.FrameIndex != frame {
				continue
			}
			switch kind {
			case "codec":
				if o.CodecError > 0 {
					return true
				}
			case "content":
				if o.ContentMarker > 0 {
					return true
				}
			case "timing":
				if o.TimingFault >= 0.75 {
					return true
				}
			}
		}
		return false
	}

	for _, event := range meta.CodecEvents {
		covered := false
		for _, o := range existing {
			if o.CodecError > 0 && codecEventMatches(o, event) {
				covered = true
				break
			}
		}
		if event.Confidence <= 0 || covered {
			continue
		}
		o := Observation{
			FrameIndex: event.FrameIndex,
			Time:       event.Time,
			SeamY:      event.Y,
			CodecError: Clamp01(event.Confidence),
			Messages:   []string{"bitstream/decoder event without matching pixel candidate"},
		}
		if event.Y > 0 {
			o.BlockAlignment = 0.9
		}
		if event.Message != "" {
			o.Messages = append(o.Messages, "decoder: "+event.Message)
		}
		out = append(out, o)
	}

	for _, window := range meta.ContentEffectWindows {
		covered := false
		for _, o := range existing {
			if o.ContentMarker > 0 && observationInWindow(o, window) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		frame := frameNearTime(meta.FrameTiming, window.StartSeconds)
		out = append(out, Observation{
			FrameIndex:    frame,
			Time:          window.StartSeconds,
			ContentMarker: Clamp01(window.Confidence),
			Messages:      []string{"content-effect window without a matching visual seam"},
		})
	}

	for frame, fault := range timingFaults {
		if fault < 0.75 || hasKey(frame, 0, "timing") {
			continue
		}
		t := 0.0
		for _, ft := range meta.FrameTiming {
			if ft.FrameIndex == frame {
				t = ft.PTSSeconds
				if t == 0 {
					t = ft.CaptureSeconds
				}
			}
		}
		out = append(out, Observation{
			FrameIndex:  frame,
			Time:        t,
			TimingFault: fault,
			Messages:    []string{"capture/PTS timing discontinuity without a localized seam"},
		})
	}
	return out
}

func timingFaultByFrame(timings []FrameTiming) map[int]float64 {
	out := map[int]float64{}
	if len(timings) == 0 {
		return out
	}
	ordered := append([]FrameTiming{}, timings...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].FrameIndex < ordered[j].FrameIndex })

	nominal := nominalInterval(ordered)
	for i, current := range ordered {
		var previous *FrameTiming
		if i > 0 {
			previous = &ordered[i-1]
		}
		expected := current.ExpectedInterval
		if expected <= 0 {
			expected = nominal
		}
		if expected <= 0 || previous == nil {
			continue
		}
		fault := 0.0
		if current.PTSSeconds > 0 && previous.PTSSeconds > 0 {
			delta := current.PTSSeconds - previous.PTSSeconds
			fault = math.Max(fault, intervalDeviation(delta, expected))
		}
		if current.CaptureSeconds > 0 && previous.CaptureSeconds > 0 {
			delta := current.CaptureSeconds - previous.CaptureSeconds
			fault = math.Max(fault, intervalDeviation(delta, expected))
			if current.PTSSeconds > 0 && previous.PTSSeconds > 0 {
				ptsDelta := current.PTSSeconds - previous.PTSSeconds
				drift := math.Abs(delta-ptsDelta) / (0.5 * expected)
				fault = math.Max(fault, Clamp01((drift-0.25)/0.75))
			}
		}
		if fault > 0 {
			out[current.FrameIndex] = Clamp01(fault)
		}
	}
	return out
}

func nominalInterval(t []FrameTiming) float64 {
	var explicit, pts, capture []float64
	for _, item := range t {
		if item.ExpectedInterval > 0 {
			explicit = append(explicit, item.ExpectedInterval)
		}
	}
	if len(explicit) > 0 {
		sort.Float64s(explicit)
		return explicit[len(explicit)/2]
	}
	for i := 1; i < len(t); i++ {
		if d := t[i].PTSSeconds - t[i-1].PTSSeconds; d > 0 {
			pts = append(pts, d)
		}
		if d := t[i].CaptureSeconds - t[i-1].CaptureSeconds; d > 0 {
			capture = append(capture, d)
		}
	}
	sort.Float64s(pts)
	sort.Float64s(capture)
	switch {
	case len(pts) > 0:
		return pts[len(pts)/2]
	case len(capture) > 0:
		return capture[len(capture)/2]
	default:
		return 0
	}
}

func intervalDeviation(delta, nominal float64) float64 {
	if delta <= 0 || nominal <= 0 {
		return 1
	}
	logDeviation := math.Abs(math.Log2(delta / nominal))
	return Clamp01((logDeviation - 0.15) / 0.8)
}

func observationInWindow(o Observation, w EffectWindow) bool {
	t := o.Time
	if t == 0 {
		return false
	}
	return t >= w.StartSeconds && t <= w.EndSeconds
}

func codecEventMatches(o Observation, e CodecEvent) bool {
	if e.FrameIndex != 0 && o.FrameIndex != 0 && e.FrameIndex == o.FrameIndex {
		return true
	}
	if e.Time != 0 && o.Time != 0 && math.Abs(e.Time-o.Time) <= 0.08 {
		return true
	}
	return false
}

func frameNearTime(timings []FrameTiming, target float64) int {
	bestFrame := 0
	bestDiff := math.MaxFloat64
	for _, t := range timings {
		ref := t.PTSSeconds
		if ref == 0 {
			ref = t.CaptureSeconds
		}
		if diff := math.Abs(ref - target); diff < bestDiff {
			bestDiff = diff
			bestFrame = t.FrameIndex
		}
	}
	return bestFrame
}

func sortFindings(items []Finding) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].FrameIndex != items[j].FrameIndex {
			return items[i].FrameIndex < items[j].FrameIndex
		}
		if items[i].SeamY != items[j].SeamY {
			return items[i].SeamY < items[j].SeamY
		}
		return items[i].ID < items[j].ID
	})
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
