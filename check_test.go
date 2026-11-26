package main

import "testing"

func TestShortAssetFirstUnplayableFrame(t *testing.T) {
	pkg, err := LoadPackage("testdata/short-asset", "")
	if err != nil {
		t.Fatalf("LoadPackage: %v", err)
	}

	report := Analyze(pkg)
	if len(report.Errors) != 0 {
		t.Fatalf("unexpected report errors: %v", report.Errors)
	}
	if len(report.Assets) != 3 {
		t.Fatalf("AssetCount = %d, want 3", len(report.Assets))
	}
	if report.FirstUnplayable == nil {
		t.Fatal("FirstUnplayable = nil, want short subtitle boundary")
	}

	got := report.FirstUnplayable
	if got.Reel != 1 {
		t.Fatalf("Reel = %d, want 1", got.Reel)
	}
	if got.OutputUnit != 119 {
		t.Errorf("OutputUnit = %d, want 119", got.OutputUnit)
	}
	if got.FirstMissingSource != 124 {
		t.Errorf("FirstMissingSource = %d, want 124", got.FirstMissingSource)
	}
	if got.CPLFrame != "119" {
		t.Errorf("CPLFrame = %q, want 119", got.CPLFrame)
	}
	if got.Timecode != "00:00:04:23" {
		t.Errorf("Timecode = %q, want 00:00:04:23", got.Timecode)
	}
	if got.Seconds != "4.958333333" {
		t.Errorf("Seconds = %q, want 4.958333333", got.Seconds)
	}
	for _, asset := range report.Assets {
		if asset.Reference == nil || len(asset.Reference.Problems) != 0 {
			t.Errorf("asset %s reference problems: %+v", asset.ID, asset.Reference)
		}
	}
}

func TestTimecodeBoundary(t *testing.T) {
	rate, err := parseRational("24 1")
	if err != nil {
		t.Fatal(err)
	}
	seconds := rate.unitsToSeconds(119)
	if got := timecode(seconds, 24); got != "00:00:04:23" {
		t.Fatalf("timecode(119 frames) = %q, want 00:00:04:23", got)
	}
}
