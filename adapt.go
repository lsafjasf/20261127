package hdrframe

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// AdaptToolJSON converts common hdr10plus_tool/SMPTE-adjacent JSON exports into
// the canonical Input. It intentionally preserves the full raw object in Values.
func AdaptToolJSON(data []byte, fps string) (Input, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return Input{}, fmt.Errorf("hdrframe: parse HDR10+ JSON: %w", err)
	}
	rootMap, _ := root.(map[string]any)
	if fps == "" {
		fps = stringFromAny(deepFind(rootMap, "FrameRate", "FPS", "frame_rate"))
		if fps == "" {
			fps = "24"
		}
	}

	rawRecords := findMapArray(rootMap, "Metadata", "DynamicMetadata", "Records", "metadata", "records")
	if rawRecords == nil {
		if arr, ok := root.([]any); ok {
			rawRecords = arr
		}
	}
	scenes := parseSceneIntervals(findMapArray(rootMap, "SceneInfo", "Scenes", "scene_info"))

	input := Input{SchemaVersion: 1, FPS: fps, Policy: DefaultPolicy()}
	frameSet := make(map[int64]bool)
	for i, item := range rawRecords {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		frame := int64FromAny(firstValue(obj,
			"SequenceFrameIndex", "sequence_frame_index", "FrameIndex", "frame_index", "Frame", "frame"))
		seq := int64FromAny(firstValue(obj,
			"SequenceNumber", "sequence_number", "Sequence", "sequence"))
		if seq == 0 {
			seq = frame
		}
		sceneFrame := int64FromAny(firstValue(obj, "SceneFrameIndex", "scene_frame_index"))
		sceneIDVal := int64FromAny(firstValue(obj, "SceneId", "SceneID", "scene_id", "SceneNumber", "scene_number"))
		if sceneIDVal == 0 {
			sceneIDVal = sceneAtFrame(scenes, frame)
		}
		version := stringFromAny(firstValue(obj, "Version", "ParameterVersion", "parameter_version", "Id", "ID"))
		if version == "" {
			version = fmt.Sprintf("hdr10plus-frame-%d", frame)
		}
		frameCopy := frame
		meta := Metadata{
			ID: version, Version: version, Sequence: seq, Frame: &frameCopy,
			SceneFrameIndex: Int64Ptr(sceneFrame), Source: "hdr10plus_tool_json",
			Values: obj,
		}
		if sceneIDVal > 0 || hasKey(obj, "SceneId", "SceneID", "scene_id") {
			meta.SceneID = Int64Ptr(sceneIDVal)
		}
		if seconds := floatFromAny(firstValue(obj, "EffectiveSeconds", "effective_seconds", "EffectiveTime")); seconds != 0 {
			meta.EffectiveSeconds = Float64Ptr(seconds)
		}
		meta.Highlight = HighlightParams{
			TargetPeakNits: floatFromAny(deepFind(obj, "TargetedSystemDisplayMaximumLuminance", "TargetPeakLuminance", "DisplayPeakLuminance")),
			PeakNits:       floatFromAny(deepFind(obj, "PeakLuminance", "MaxLuminance", "MasteringDisplayPeakLuminance")),
			Gain:           floatFromAny(deepFind(obj, "HighlightGain", "gain")),
		}
		meta.Color = ColorParams{
			HueShiftDegrees: floatFromAny(deepFind(obj, "HueShiftDegrees", "HueShift", "hue_shift_degrees")),
			Saturation:      floatFromAny(deepFind(obj, "Saturation", "saturation")),
		}
		if meta.EffectiveSeconds == nil {
			fpsFloat, _ := parseFPS(fps)
			meta.EffectiveSeconds = Float64Ptr(float64(frame) / fpsFloat)
		}
		input.Metadata = append(input.Metadata, meta)
		frameSet[frame] = true
		_ = i
	}

	for frame := range frameSet {
		input.Targets = append(input.Targets, Target{Frame: frame})
	}
	sort.Slice(input.Targets, func(i, j int) bool { return input.Targets[i].Frame < input.Targets[j].Frame })
	return input, nil
}

// DefaultPolicy returns the standard tolerant hold-last policy.
func DefaultPolicy() Policy {
	return Policy{Duplicate: "latest", MissingMetadata: "hold_last", SequenceGap: "accept", TimeToleranceSecs: 1e-6}
}

type sceneInterval struct {
	start int64
	id    int64
}

func parseSceneIntervals(raw []any) []sceneInterval {
	var scenes []sceneInterval
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := int64FromAny(firstValue(obj, "SceneId", "SceneID", "Id", "ID", "scene_id"))
		start := int64FromAny(firstValue(obj, "FirstFrameIndex", "StartFrame", "SequenceFrameIndex", "SceneFrameIndex", "first_frame"))
		scenes = append(scenes, sceneInterval{start: start, id: id})
	}
	sort.Slice(scenes, func(i, j int) bool { return scenes[i].start < scenes[j].start })
	return scenes
}

func sceneAtFrame(scenes []sceneInterval, frame int64) int64 {
	var id int64
	for _, scene := range scenes {
		if scene.start <= frame {
			id = scene.id
		} else {
			break
		}
	}
	return id
}

func normalizeKey(key string) string {
	key = strings.ToLower(key)
	for _, ch := range []string{"_", "-", " ", "."} {
		key = strings.ReplaceAll(key, ch, "")
	}
	return key
}

func normalizedMap(obj map[string]any) map[string]any {
	out := make(map[string]any, len(obj))
	for key, value := range obj {
		out[normalizeKey(key)] = value
	}
	return out
}

func firstValue(obj map[string]any, keys ...string) any {
	normalized := normalizedMap(obj)
	for _, key := range keys {
		if value, ok := normalized[normalizeKey(key)]; ok {
			return value
		}
	}
	return nil
}

func hasKey(obj map[string]any, keys ...string) bool {
	return firstValue(obj, keys...) != nil
}

func findMapArray(root map[string]any, keys ...string) []any {
	if root == nil {
		return nil
	}
	normalized := normalizedMap(root)
	for _, key := range keys {
		if value, ok := normalized[normalizeKey(key)]; ok {
			if arr, ok := value.([]any); ok {
				return arr
			}
		}
	}
	return nil
}

func deepFind(value any, keys ...string) any {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[normalizeKey(key)] = true
	}
	var found any
	var walk func(any)
	walk = func(node any) {
		if found != nil {
			return
		}
		switch typed := node.(type) {
		case map[string]any:
			for key, child := range typed {
				if wanted[normalizeKey(key)] {
					found = child
					return
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return found
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		n, _ := typed.Int64()
		return n
	case string:
		var n int64
		fmt.Sscanf(strings.TrimSpace(typed), "%d", &n)
		return n
	}
	return 0
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		var n float64
		fmt.Sscanf(strings.TrimSpace(typed), "%f", &n)
		return n
	}
	return 0
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%v", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	default:
		return ""
	}
}
