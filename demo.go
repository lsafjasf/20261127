package laddercheck

import (
	"os"
	"path/filepath"
)

// DemoOptions controls the built-in synthetic incident ladder.
type DemoOptions struct {
	Dir string
}

// DemoSpecs returns the five-rung reproduction:
//
//	1080p aligned reference; 720p empty-edit shift (remux);
//	480p initial PTS offset (remux); 360p wrong GOP cadence (re-encode);
//	240p open-GOP/CRA keyframes that are not IDR (re-encode).
func DemoSpecs() []FixtureSpec {
	key60 := []int{0, 60, 120}
	return []FixtureSpec{
		{
			ID: "rung-1080p", Role: "1920x1080", Width: 1920, Height: 1080,
			MovieTimescale: 1000, MediaTimescale: 30000, Frames: 150, FrameDurationTicks: 1000,
			KeyframeSamples: key60,
		},
		{
			ID: "rung-720p", Role: "1280x720", Width: 1280, Height: 720,
			MovieTimescale: 90000, MediaTimescale: 90000, Frames: 150, FrameDurationTicks: 3000,
			KeyframeSamples: key60,
			Edits: []FixtureEdit{
				{Empty: true, SegmentDurationTicks: 3000}, // one frame of empty lead
				{SegmentDurationTicks: 450000, MediaTimeTicks: 0},
			},
		},
		{
			ID: "rung-480p", Role: "854x480", Width: 854, Height: 480,
			MovieTimescale: 1000, MediaTimescale: 15360, Frames: 150, FrameDurationTicks: 512,
			CTOConstant:     1536, // 3 frames == 0.1 s initial PTS offset, no elst
			KeyframeSamples: key60,
		},
		{
			ID: "rung-360p", Role: "640x360", Width: 640, Height: 360,
			MovieTimescale: 1000, MediaTimescale: 30000, Frames: 150, FrameDurationTicks: 1000,
			KeyframeSamples: []int{0, 48, 96, 144}, // 1.6 s GOP cadence
		},
		{
			ID: "rung-240p", Role: "426x240", Width: 426, Height: 240,
			MovieTimescale: 1000, MediaTimescale: 30000, Frames: 150, FrameDurationTicks: 1000,
			KeyframeSamples: key60,
			NonIDR:          map[int]bool{60: true, 120: true}, // CRA/open-GOP at grid points
		},
	}
}

// DemoConstraint is the explicit unified target used by the reproduction.
func DemoConstraint() ConstraintOptions {
	return ConstraintOptions{
		FPS:            NewRat(30, 1),
		GOPFrames:      60,
		GOPSeconds:     NewRat(2, 1),
		HorizonSeconds: NewRat(5, 1),
	}
}

// GenerateDemo writes deterministic MP4 rungs into dir and evaluates them.
func GenerateDemo(dir string) (*Report, []string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	var paths []string
	var facts []RenditionFacts
	for _, spec := range DemoSpecs() {
		data, err := BuildFixtureMP4(spec)
		if err != nil {
			return nil, nil, err
		}
		path := filepath.Join(dir, spec.ID+".mp4")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, nil, err
		}
		paths = append(paths, path)

		f, err := ParseMP4File(path)
		if err != nil {
			return nil, nil, err
		}
		f.ID = spec.ID
		f.Role = spec.Role
		facts = append(facts, *f)
	}
	report := EvaluateLadder("built-in-demo:"+dir, facts, DemoConstraint())
	return report, paths, nil
}
