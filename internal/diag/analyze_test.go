package diag

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lsafjasf/20261127/internal/testmedia"
)

func TestProfile81FullAndBaseFallback(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:         "hvc1",
		DVProfile:         8,
		DVCCID:            1,
		RPUProfile:        8,
		UsePrevAfterFirst: true,
	}, []DeviceCapabilities{
		{
			Name: "dv-tv", DolbyVision: true, Profiles: []int{8}, MaxLevel: 9,
			CompatibilityIDs: []int{1}, SampleEntries: []string{"hvc1"},
			BaseFormats: []string{"hdr10"}, HEVCMain10: true, MaxHEVCLevelIDC: 153,
		},
		{
			Name: "hdr10-tv", SampleEntries: []string{"hvc1"},
			BaseFormats: []string{"hdr10"}, HEVCMain10: true, MaxHEVCLevelIDC: 153,
		},
	})

	if !seg.FullDolbyVision.Usable || seg.FullDolbyVision.Status != "full_dv" {
		t.Fatalf("full DV = %+v", seg.FullDolbyVision)
	}
	if seg.MinimumAvailability.Status != "base_layer" || seg.MinimumAvailability.Mode != "hdr10" {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
	if len(seg.Devices) != 2 {
		t.Fatalf("devices = %+v", seg.Devices)
	}
	if seg.Devices[0].Status != "full_dv" {
		t.Fatalf("DV device = %+v", seg.Devices[0])
	}
	if seg.Devices[1].Status != "base_layer" || !seg.Devices[1].Playable {
		t.Fatalf("HDR10 device = %+v", seg.Devices[1])
	}
}

func TestProfile5IsDVOnly(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:  "dvh1",
		DVProfile:  5,
		RPUProfile: 5,
		DVRPU:      true,
		DVBL:       true,
	}, []DeviceCapabilities{
		{
			Name: "dv-phone", DolbyVision: true, Profiles: []int{5}, MaxLevel: 9,
			CompatibilityIDs: []int{0}, SampleEntries: []string{"dvh1"},
			BaseFormats: []string{"hdr10"}, HEVCMain10: true,
		},
		{
			Name: "hdr10-box", SampleEntries: []string{"dvh1"},
			BaseFormats: []string{"hdr10"}, HEVCMain10: true, DVEntryBaseFallback: true,
		},
	})

	if !seg.FullDolbyVision.Usable {
		t.Fatalf("full DV = %+v; issues=%+v", seg.FullDolbyVision, seg.Issues)
	}
	if seg.MinimumAvailability.Status != "dv_only" || !seg.MinimumAvailability.Usable {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
	if seg.Devices[0].Status != "full_dv" {
		t.Fatalf("DV device = %+v", seg.Devices[0])
	}
	if seg.Devices[1].Status != "black" {
		t.Fatalf("non-DV device = %+v", seg.Devices[1])
	}
}

func TestMissingDVConfig(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:  "dvh1",
		NoDV:       true,
		RPUProfile: 8,
	}, nil)

	if seg.DVConfig != nil {
		t.Fatal("unexpected DV config")
	}
	if !hasIssue(seg, CategoryContainer, "missing_dv_config") {
		t.Fatalf("issues = %+v", seg.Issues)
	}
	if seg.MinimumAvailability.Usable {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
}

func TestMissingDVConfigLeavesCompatibleBase(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:  "hvc1",
		NoDV:       true,
		RPUProfile: 8,
	}, nil)

	if !hasIssue(seg, CategoryContainer, "missing_dv_config") {
		t.Fatalf("issues = %+v", seg.Issues)
	}
	if seg.MinimumAvailability.Status != "base_layer" || seg.MinimumAvailability.Mode != "hdr10" {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
}

func TestRPUProfileContradictionStillAllowsBase(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:  "hvc1",
		DVProfile:  8,
		DVCCID:     1,
		RPUProfile: 5,
	}, nil)

	if !hasIssue(seg, CategoryConflict, "rpu_profile_conflict") {
		t.Fatalf("issues = %+v", seg.Issues)
	}
	if seg.FullDolbyVision.Usable {
		t.Fatalf("full DV should be blocked: %+v", seg.FullDolbyVision)
	}
	if seg.MinimumAvailability.Status != "base_layer" || seg.MinimumAvailability.Mode != "hdr10" {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
}

func TestProfile7EnhancementAndHDR10Fallback(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:    "dvhe",
		DVProfile:    7,
		DVCCID:       6,
		RPUProfile:   7,
		IncludeHVCE:  true,
		HVCELevelIDC: 153,
	}, []DeviceCapabilities{
		{
			Name: "full-dv", DolbyVision: true, Profiles: []int{7}, MaxLevel: 9,
			CompatibilityIDs: []int{6}, SupportsEnhancementLayer: true,
			SampleEntries: []string{"dvhe"}, BaseFormats: []string{"hdr10"},
			HEVCMain10: true,
		},
		{
			Name: "hdr10-fallback", SampleEntries: []string{"dvhe"},
			DVEntryBaseFallback: true, BaseFormats: []string{"hdr10"},
			HEVCMain10: true, MaxHEVCLevelIDC: 153,
		},
	})

	if !seg.Enhancement.HVCETrackPresent {
		t.Fatal("hvcE was not parsed")
	}
	if !seg.FullDolbyVision.Usable {
		t.Fatalf("full DV = %+v; issues=%+v", seg.FullDolbyVision, seg.Issues)
	}
	if seg.MinimumAvailability.Mode != "hdr10" {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
	if seg.Devices[0].Status != "full_dv" {
		t.Fatalf("DV device = %+v", seg.Devices[0])
	}
	if seg.Devices[1].Status != "base_layer" {
		t.Fatalf("fallback device = %+v", seg.Devices[1])
	}
}

func TestProfile7WithoutELIsBaseOnly(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:  "dvhe",
		DVProfile:  7,
		DVCCID:     6,
		RPUProfile: 7,
		DVRPU:      true,
		DVBL:       true,
		DVEL:       false,
	}, nil)

	if !hasIssue(seg, CategoryConflict, "missing_el_declaration") {
		t.Fatalf("issues = %+v", seg.Issues)
	}
	if seg.MinimumAvailability.Status != "base_layer" || seg.MinimumAvailability.Mode != "hdr10" {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
}

func TestAttachmentOnlyTrackIsNotStandalone(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:  "dvhe",
		DVProfile:  7,
		DVCCID:     6,
		RPUProfile: 7,
		DVRPU:      true,
		DVBL:       false,
		DVEL:       true,
	}, nil)

	if seg.FullDolbyVision.Status != "not_standalone" ||
		seg.MinimumAvailability.Status != "not_standalone" {
		t.Fatalf("full=%+v minimum=%+v", seg.FullDolbyVision, seg.MinimumAvailability)
	}
}

func TestFirstRPUReferenceNeedsAnchor(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:       "hvc1",
		DVProfile:       8,
		DVCCID:          1,
		RPUProfile:      8,
		FirstRPUUsePrev: true,
	}, nil)

	if !hasIssue(seg, CategoryConflict, "rpu_reference_missing") {
		t.Fatalf("issues = %+v", seg.Issues)
	}
	if seg.RPU == nil || seg.RPU.ReferencesOK {
		t.Fatalf("RPU = %+v", seg.RPU)
	}
}

func TestLimitedRPUScanIsConditional(t *testing.T) {
	data := testmedia.BuildMP4(testmedia.MP4Options{
		EntryType:   "hvc1",
		DVProfile:   8,
		DVCCID:      1,
		RPUProfile:  8,
		SampleCount: 5,
	})
	path := filepath.Join(t.TempDir(), "limited.mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report := AnalyzeFile(path, nil, Options{ScanSamples: 2})
	if report.Error != "" || len(report.Segments) != 1 {
		t.Fatalf("report = %+v", report)
	}
	seg := report.Segments[0]
	if seg.FullDolbyVision.Status != "conditional" {
		t.Fatalf("full = %+v", seg.FullDolbyVision)
	}
}

func TestEncryptedTrackIsNotBypassed(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:  "dvh1",
		DVProfile:  5,
		RPUProfile: 5,
		Encrypted:  true,
	}, nil)

	if !seg.Container.Encrypted || seg.Container.OriginalFormat != "dvh1" {
		t.Fatalf("container = %+v", seg.Container)
	}
	if seg.RPU != nil {
		t.Fatalf("encrypted samples must not be scanned: %+v", seg.RPU)
	}
	if seg.MinimumAvailability.Status != "auth_required" {
		t.Fatalf("minimum = %+v", seg.MinimumAvailability)
	}
	if !hasIssue(seg, CategoryDRM, "encrypted_video") {
		t.Fatalf("issues = %+v", seg.Issues)
	}
}

func TestSPSLevelContradiction(t *testing.T) {
	seg := analyzeFixture(t, testmedia.MP4Options{
		EntryType:    "hvc1",
		DVProfile:    8,
		DVCCID:       1,
		RPUProfile:   8,
		HVCCLevelIDC: 153,
		SPSLevelIDC:  120,
	}, nil)

	if !hasIssue(seg, CategoryConflict, "hevc_level_conflict") {
		t.Fatalf("issues = %+v", seg.Issues)
	}
}

func analyzeFixture(t *testing.T, opts testmedia.MP4Options, devices []DeviceCapabilities) Segment {
	t.Helper()
	data := testmedia.BuildMP4(opts)
	path := filepath.Join(t.TempDir(), "fixture.mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report := AnalyzeFile(path, devices, Options{})
	if report.Error != "" {
		t.Fatalf("analyze: %s", report.Error)
	}
	if len(report.Segments) != 1 {
		t.Fatalf("segments = %+v", report.Segments)
	}
	return report.Segments[0]
}

func hasIssue(seg Segment, category, code string) bool {
	for _, issue := range seg.Issues {
		if issue.Category == category && issue.Code == code {
			return true
		}
	}
	return false
}
