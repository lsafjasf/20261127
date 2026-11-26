package gamutcompare

import (
	"encoding/json"
	"fmt"
)

type fileInput struct {
	DirectionCenter string          `json:"direction_center"`
	Target          json.RawMessage `json:"target"`
	Displays        []displayInput  `json:"displays"`
}

type displayInput struct {
	Name  string `json:"name"`
	Red   *Point `json:"red"`
	Green *Point `json:"green"`
	Blue  *Point `json:"blue"`
	White *Point `json:"white"`
}

type targetInput struct {
	Name   string  `json:"name"`
	Preset string  `json:"preset"`
	Red    *Point  `json:"red"`
	Green  *Point  `json:"green"`
	Blue   *Point  `json:"blue"`
	Points []Point `json:"points"`
	White  *Point  `json:"white"`
}

// ParseInput parses the documented JSON input.
func ParseInput(data []byte) ([]DisplaySpec, TargetSpec, Options, error) {
	var file fileInput
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, TargetSpec{}, Options{}, fmt.Errorf("invalid input JSON: %w", err)
	}
	if len(file.Displays) == 0 {
		return nil, TargetSpec{}, Options{}, fmt.Errorf("input requires at least one display")
	}

	target, err := parseTarget(file.Target)
	if err != nil {
		return nil, TargetSpec{}, Options{}, err
	}

	displays := make([]DisplaySpec, 0, len(file.Displays))
	for i, input := range file.Displays {
		name := input.Name
		if name == "" {
			name = fmt.Sprintf("display-%d", i+1)
		}
		if input.Red == nil || input.Green == nil || input.Blue == nil {
			return nil, TargetSpec{}, Options{}, fmt.Errorf("display %q requires red, green, and blue coordinates", name)
		}
		displays = append(displays, DisplaySpec{
			Name:  name,
			Red:   *input.Red,
			Green: *input.Green,
			Blue:  *input.Blue,
			White: input.White,
		})
	}

	options := Options{CenterMode: file.DirectionCenter}
	if options.CenterMode == "" {
		options.CenterMode = "display_white"
	}
	return displays, target, options, nil
}

func parseTarget(raw json.RawMessage) (TargetSpec, error) {
	if len(raw) == 0 {
		return TargetSpec{}, fmt.Errorf("input requires a target gamut")
	}

	var preset string
	if err := json.Unmarshal(raw, &preset); err == nil {
		return TargetPreset(preset)
	}

	var input targetInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return TargetSpec{}, fmt.Errorf("target must be a preset name or object: %w", err)
	}

	var target TargetSpec
	if input.Preset != "" {
		presetTarget, err := TargetPreset(input.Preset)
		if err != nil {
			return TargetSpec{}, err
		}
		target = presetTarget
	}

	switch {
	case len(input.Points) >= 3:
		target.Polygon = input.Points
	case input.Red != nil && input.Green != nil && input.Blue != nil:
		target.Polygon = []Point{*input.Red, *input.Green, *input.Blue}
	}
	if len(target.Polygon) < 3 {
		return TargetSpec{}, fmt.Errorf("target requires preset or at least 3 points/RGB primaries")
	}
	if input.Name != "" {
		target.Name = input.Name
	}
	if input.White != nil {
		target.White = input.White
	}
	if target.Name == "" {
		target.Name = "target"
	}
	return target, nil
}
