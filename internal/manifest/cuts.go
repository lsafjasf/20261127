package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const defaultCutToleranceMS int64 = 2

func NormalizeCutSource(cuts CutsConfig) string {
	source := strings.ToLower(strings.TrimSpace(cuts.Source))
	if source != "" {
		return source
	}
	if cuts.Path != "" || len(cuts.Points) > 0 {
		return "list"
	}
	return "paired"
}

func CutToleranceMS(cuts CutsConfig) int64 {
	if cuts.ToleranceMS > 0 {
		return cuts.ToleranceMS
	}
	return defaultCutToleranceMS
}

func LoadCutPoints(cuts CutsConfig) ([]CutPoint, error) {
	if NormalizeCutSource(cuts) != "list" {
		return nil, nil
	}

	points := append([]CutPoint(nil), cuts.Points...)
	if cuts.Path != "" {
		data, err := os.ReadFile(cuts.Path)
		if err != nil {
			return nil, fmt.Errorf("read cuts: %w", err)
		}
		loaded, err := decodeCutPoints(data)
		if err != nil {
			return nil, fmt.Errorf("decode cuts %s: %w", cuts.Path, err)
		}
		points = loaded
	}
	if err := validateCutPoints(points); err != nil {
		return nil, err
	}
	sort.SliceStable(points, func(i, j int) bool {
		return points[i].TimeMS < points[j].TimeMS
	})
	return points, nil
}

func decodeCutPoints(data []byte) ([]CutPoint, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("cut list is empty")
	}
	if trimmed[0] == '[' {
		var points []CutPoint
		if err := json.Unmarshal(trimmed, &points); err != nil {
			return nil, err
		}
		return points, nil
	}

	var document struct {
		Cuts []CutPoint `json:"cuts"`
	}
	if err := json.Unmarshal(trimmed, &document); err != nil {
		return nil, err
	}
	return document.Cuts, nil
}

func validateCutPoints(points []CutPoint) error {
	if len(points) == 0 {
		return fmt.Errorf("cut list must contain at least one cut")
	}
	ids := make(map[string]struct{}, len(points))
	times := make(map[int64]struct{}, len(points))
	for i, point := range points {
		if strings.TrimSpace(point.ID) == "" {
			return fmt.Errorf("cuts[%d].id is required", i)
		}
		if point.TimeMS < 0 {
			return fmt.Errorf("cut %q time_ms must not be negative", point.ID)
		}
		if _, exists := ids[point.ID]; exists {
			return fmt.Errorf("duplicate cut id %q", point.ID)
		}
		if _, exists := times[point.TimeMS]; exists {
			return fmt.Errorf("duplicate cut time %d ms", point.TimeMS)
		}
		ids[point.ID] = struct{}{}
		times[point.TimeMS] = struct{}{}
	}
	return nil
}

func CutMarkersFromProbe(probe ProbeData) ([]CutMarker, int) {
	markers := make([]CutMarker, 0, len(probe.Chapters))
	malformed := 0
	for i, chapter := range probe.Chapters {
		timeMS, ok := SecondsToMS(chapter.StartTime)
		if !ok {
			malformed++
			continue
		}
		id := strings.TrimSpace(string(chapter.ID))
		if id == "" {
			id = fmt.Sprintf("chapter-%d", i+1)
		}
		markers = append(markers, CutMarker{
			ID:     id,
			Label:  chapterTag(chapter.Tags, "title"),
			TimeMS: timeMS,
		})
	}
	sort.SliceStable(markers, func(i, j int) bool {
		return markers[i].TimeMS < markers[j].TimeMS
	})
	return markers, malformed
}

func chapterTag(tags map[string]string, name string) string {
	for key, value := range tags {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func MatchCuts(expected []CutPoint, actual []CutMarker, toleranceMS int64) (missing []CutPoint, unexpected []CutMarker) {
	used := make(map[int]bool, len(actual))
	for _, point := range expected {
		best := -1
		bestDistance := int64(0)
		for i, marker := range actual {
			if used[i] {
				continue
			}
			distance := marker.TimeMS - point.TimeMS
			if distance < 0 {
				distance = -distance
			}
			if distance <= toleranceMS && (best == -1 || distance < bestDistance) {
				best = i
				bestDistance = distance
			}
		}
		if best == -1 {
			missing = append(missing, point)
			continue
		}
		used[best] = true
	}
	for i, marker := range actual {
		if !used[i] {
			unexpected = append(unexpected, marker)
		}
	}
	return missing, unexpected
}
