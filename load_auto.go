package hdrframe

import (
	"encoding/json"
	"os"
)

// LoadSampleFileAuto reads canonical JSON or a common HDR10+ tool export.
func LoadSampleFileAuto(path, format, fps string) (SampleFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SampleFile{}, err
	}
	if format == "hdr10plus" || format == "hdr10plus-json" {
		input, err := AdaptToolJSON(data, fps)
		if err != nil {
			return SampleFile{}, err
		}
		return SampleFile{SchemaVersion: 1, Samples: []Input{input}}, nil
	}
	if format == "canonical" {
		return LoadSampleFile(path)
	}

	var root map[string]any
	_ = json.Unmarshal(data, &root)
	normalized := normalizedMap(root)
	_, hasSamples := normalized["samples"]
	_, hasTargets := normalized["targets"]
	_, hasFrames := normalized["frames"]
	_, hasPlayer := normalized["playerevents"]
	_, hasPlayerLog := normalized["playerlog"]
	if hasSamples || hasTargets || hasFrames || hasPlayer || hasPlayerLog {
		return LoadSampleFile(path)
	}
	input, err := AdaptToolJSON(data, fps)
	if err != nil {
		return SampleFile{}, err
	}
	return SampleFile{SchemaVersion: 1, Samples: []Input{input}}, nil
}
