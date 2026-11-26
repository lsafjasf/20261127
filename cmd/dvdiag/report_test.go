package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lsafjasf/20261127/internal/diag"
)

func TestTextReportIncludesThreeDecisionLayers(t *testing.T) {
	report := diag.Report{Files: []diag.FileReport{{
		File: "sample.mp4",
		Segments: []diag.Segment{{
			TrackID: 1, SampleDescription: 1, SampleEntry: "hvc1", Codec: "hevc",
			Width: 3840, Height: 2160, SampleCount: 24, FPS: 23.976,
			Container: diag.ContainerInfo{Format: "isobmff", MajorBrand: "isom"},
			DVConfig: &diag.DVConfig{
				Box: "dvvC", Profile: 8, Level: 9, RPUPresent: true, BLPresent: true,
				CompatibilityID: 1, CompatibilityFormat: "HDR10/PQ10",
			},
			Issues: []diag.Issue{{
				Category: diag.CategoryConflict, Code: "rpu_profile_conflict",
				Severity: diag.SeverityError, Message: "profile mismatch",
			}},
			FullDolbyVision: diag.Availability{
				Status: "blocked", Mode: "dolby_vision", Reason: "profile mismatch",
			},
			MinimumAvailability: diag.Availability{
				Status: "base_layer", Mode: "hdr10", Usable: true, Reason: "fallback",
			},
			Devices: []diag.DeviceResult{{
				Device: "tv", Status: "base_layer", Mode: "hdr10", Playable: true,
				Reason: "fallback",
			}},
		}},
	}}}
	var out bytes.Buffer
	writeTextTo(&out, report)
	text := out.String()
	for _, want := range []string{"SEGMENT", "bitstream_conflict", "full-dv: blocked", "minimum: base_layer", "device tv: base_layer"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}
