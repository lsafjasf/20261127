//go:build cgo

package iccproof

import (
	"os"
	"testing"
)

func TestLCMSRealICCProfilesAndGamutAlarm(t *testing.T) {
	srcPath := "/usr/share/color/icc/ghostscript/a98.icc"
	rgbMediaPath := "/usr/share/color/icc/ghostscript/srgb.icc"
	cmykMediaPath := "/usr/share/color/icc/ghostscript/ps_cmyk.icc"
	if _, err := os.Stat(srcPath); err != nil {
		t.Skip("Ghostscript ICC fixtures are not installed")
	}
	if _, err := os.Stat(rgbMediaPath); err != nil {
		t.Skip("Ghostscript CMYK fixture is not installed")
	}
	report, err := Proof(Request{
		Source:  ProfileRef{Path: srcPath},
		Media:   ProfileRef{Path: rgbMediaPath},
		Intents: AllProofIntents(),
		Samples: []Sample{
			{ID: "wide-green", RGB: []float64{0, 1, 0}},
			{ID: "gray", RGB: []float64{0.5, 0.5, 0.5}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Config.Engine != "littlecms2" {
		t.Fatalf("engine = %q", report.Config.Engine)
	}
	if report.SourceProfile.SHA256 == "" || report.MediaProfile.TagCount == 0 {
		t.Fatalf("profile provenance incomplete: %+v %+v", report.SourceProfile, report.MediaProfile)
	}
	for _, result := range report.Results {
		if result.DeltaE00 == nil {
			t.Fatalf("%s/%s missing delta E: %s", result.SampleID, result.Intent, result.Error)
		}
		if result.MediaColorSpace != "RGB" || len(result.MediaCodes) != 3 {
			t.Fatalf("media output = %s/codes %v", result.MediaColorSpace, result.MediaCodes)
		}
		if result.SampleID == "wide-green" && !result.Boundary.MediaGamutAlarm {
			t.Fatalf("wide green %s did not trigger gamut alarm: %+v", result.Intent, result.Boundary)
		}
	}

	if _, err := os.Stat(cmykMediaPath); err != nil {
		t.Skip("Ghostscript CMYK fixture is not installed")
	}
	cmykReport, err := Proof(Request{
		Source: ProfileRef{Path: srcPath}, Media: ProfileRef{Path: cmykMediaPath},
		Intents: []Intent{IntentRelativeColorimetric},
		Samples: []Sample{{ID: "green", RGB: []float64{0, 1, 0}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := cmykReport.Results[0]
	if r.MediaColorSpace != "CMYK" || len(r.MediaCodes) != 4 || r.DeltaE00 == nil {
		t.Fatalf("CMYK proof incomplete: %+v", r)
	}
}

func TestLCMSRejectsGarbageProfile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.icc"
	if err := os.WriteFile(path, []byte("not an icc profile at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, media := testProfiles()
	_, err := Proof(Request{Source: ProfileRef{Path: path}, Media: media, Intents: []Intent{IntentPerceptual}, Samples: []Sample{{RGB: []float64{0, 0, 0}}}})
	if err == nil {
		t.Fatal("garbage ICC must fail")
	}
}

func TestLCMSDescriptorSourceToICCProfile(t *testing.T) {
	mediaPath := "/usr/share/color/icc/ghostscript/srgb.icc"
	if _, err := os.Stat(mediaPath); err != nil {
		t.Skip("Ghostscript sRGB fixture is not installed")
	}
	source, _ := testProfiles()
	report, err := Proof(Request{
		Source: source, Media: ProfileRef{Path: mediaPath},
		Intents: []Intent{IntentPerceptual, IntentAbsoluteColorimetric},
		Samples: []Sample{{ID: "green", RGB: []float64{0, 1, 0}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("results=%d", len(report.Results))
	}
	for _, result := range report.Results {
		if result.DeltaE00 == nil || result.Error != "" {
			t.Fatalf("bad result: %+v", result)
		}
	}
}
