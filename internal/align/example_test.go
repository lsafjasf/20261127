package align

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

func TestFourCameraExample(t *testing.T) {
	data, err := os.ReadFile("../../examples/four-camera.json")
	if err != nil {
		t.Fatal(err)
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatal(err)
	}
	report, err := Solve(session)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Devices) != 5 {
		t.Fatalf("device count = %d, want master + 4 cameras", len(report.Devices))
	}
	b1 := findSegment(t, report, "cam-b", "cam-b-1")
	b2 := findSegment(t, report, "cam-b", "cam-b-2")
	if b1.Status != "locked" || b2.Status != "locked" {
		t.Fatalf("restarted camera segments are not locked")
	}
	d := findSegment(t, report, "cam-d", "cam-d-1")
	if d.Status != "locked" || d.FirstTrusted == nil || d.FirstTrusted.Kind != KindFlash {
		t.Fatalf("flash-only camera did not lock: %+v", d)
	}
	c := findSegment(t, report, "cam-c", "cam-c-1")
	if len(c.Indeterminate) == 0 {
		t.Fatalf("example should demonstrate an indeterminate tail")
	}
	for _, device := range report.Devices {
		for _, segment := range device.Segments {
			assertComplementaryCoverage(t, segment)
		}
	}
}

type coverageItem struct {
	start float64
	end   float64
	kind  string
}

func assertComplementaryCoverage(t *testing.T, segment SegmentReport) {
	t.Helper()
	var items []coverageItem
	for _, piece := range segment.Map {
		items = append(items, coverageItem{start: piece.LocalStart, end: piece.LocalEnd, kind: "map"})
	}
	for _, interval := range segment.Indeterminate {
		items = append(items, coverageItem{
			start: interval.LocalStart, end: interval.LocalEnd, kind: "indeterminate",
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].start == items[j].start {
			return items[i].end < items[j].end
		}
		return items[i].start < items[j].start
	})
	cursor := segment.LocalStart
	for _, item := range items {
		if math.Abs(item.start-cursor) > 1e-6 {
			t.Fatalf("%s/%s has a %s gap/overlap at %.6f",
				segment.Device, segment.ID, item.kind, cursor)
		}
		if item.end <= item.start {
			t.Fatalf("%s/%s has an empty %s interval", segment.Device, segment.ID, item.kind)
		}
		cursor = item.end
	}
	if math.Abs(cursor-segment.LocalEnd) > 1e-6 {
		t.Fatalf("%s/%s coverage ends at %.6f, want %.6f",
			segment.Device, segment.ID, cursor, segment.LocalEnd)
	}
}
