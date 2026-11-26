package iccproof

import (
	"errors"
	"testing"
)

func testProfiles() (ProfileRef, ProfileRef) {
	bt2020 := &ColorProfile{
		ID: "explicit-bt2020", ColorSpace: "RGB", Gamma: 2.2,
		Red:   XYY{X: 0.708, Y: 0.292},
		Green: XYY{X: 0.170, Y: 0.797},
		Blue:  XYY{X: 0.131, Y: 0.046},
		White: D65XYY(),
	}
	srgb := &ColorProfile{
		ID: "explicit-srgb-not-a-default", ColorSpace: "RGB", Gamma: 2.2,
		Red:   XYY{X: 0.640, Y: 0.330},
		Green: XYY{X: 0.300, Y: 0.600},
		Blue:  XYY{X: 0.150, Y: 0.060},
		White: D65XYY(),
	}
	return ProfileRef{Descriptor: bt2020}, ProfileRef{Descriptor: srgb}
}

func TestMissingProfilesNeverUseSRGB(t *testing.T) {
	source, media := testProfiles()
	_, err := Proof(Request{Media: media, Intents: AllProofIntents(), Samples: []Sample{{RGB: []float64{0, 1, 0}}}})
	if !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("missing source error = %v, want ErrProfileRequired", err)
	}
	if err == nil || !contains(err.Error(), "refusing to use a default sRGB") {
		t.Fatalf("error must explicitly reject sRGB fallback: %v", err)
	}

	missing := ProfileRef{Path: "/definitely/not/here/profile.icc"}
	_, err = Proof(Request{Source: source, Media: missing, Intents: AllProofIntents(), Samples: []Sample{{RGB: []float64{0, 1, 0}}}})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("missing media file error = %v, want ErrProfileNotFound", err)
	}
}

func TestEmptyIntentIsRejected(t *testing.T) {
	source, media := testProfiles()
	_, err := Proof(Request{Source: source, Media: media, Samples: []Sample{{RGB: []float64{0, 0, 0}}}})
	if !errors.Is(err, ErrInvalidIntent) {
		t.Fatalf("empty intents error = %v, want ErrInvalidIntent", err)
	}
}

func TestOptionalExplicitDisplaySimulation(t *testing.T) {
	source, media := testProfiles()
	display := ProfileRef{Descriptor: &ColorProfile{
		ID: "explicit-display", ColorSpace: "RGB", Gamma: 2.2, White: D65XYY(),
		Red: XYY{X: 0.640, Y: 0.330}, Green: XYY{X: 0.300, Y: 0.600}, Blue: XYY{X: 0.150, Y: 0.060},
	}}
	report, err := Proof(Request{
		Source: source, Media: media, Display: &display,
		Intents: []Intent{IntentRelativeColorimetric},
		Samples: []Sample{{ID: "gray", RGB: []float64{0.5, 0.5, 0.5}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := report.Results[0]
	if len(result.DisplayRGB) != 3 || len(result.DisplayCodes) != 3 {
		t.Fatalf("display simulation missing: rgb=%v codes=%v", result.DisplayRGB, result.DisplayCodes)
	}
	if report.DisplayProfile == nil {
		t.Fatal("display profile provenance missing")
	}
}

func TestBoundaryDistinguishesSourceAndDevice(t *testing.T) {
	source, media := testProfiles()
	req := Request{
		Source: source, Media: media, Intents: AllProofIntents(),
		Samples: []Sample{
			{ID: "wide-green", RGB: []float64{0, 1, 0}},
			{ID: "gray", RGB: []float64{0.5, 0.5, 0.5}},
			{ID: "bad-source", RGB: []float64{0, 1.25, 0}},
		},
	}
	report, err := Proof(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 9 {
		t.Fatalf("results = %d, want 9", len(report.Results))
	}
	if len(report.IntentComparisons) != 3 {
		t.Fatalf("intent comparisons = %d, want 3 pairs", len(report.IntentComparisons))
	}

	byKey := map[string]SampleResult{}
	for _, result := range report.Results {
		byKey[result.SampleID+"/"+string(result.Intent)] = result
	}

	bad := byKey["bad-source/perceptual"]
	if bad.Boundary.Classification != ClassSourceUnrepresentable {
		t.Fatalf("bad source class = %q flags=%v", bad.Boundary.Classification, bad.Boundary.Flags)
	}
	if bad.DeltaE00 == nil || bad.DeltaEReliable == nil || *bad.DeltaEReliable {
		t.Fatalf("source-unrepresentable sample should carry an extrapolated but marked delta E: de=%v reliable=%v", bad.DeltaE00, bad.DeltaEReliable)
	}

	for _, intent := range []Intent{IntentRelativeColorimetric, IntentAbsoluteColorimetric} {
		green := byKey["wide-green/"+string(intent)]
		if green.Boundary.Classification != ClassDeviceClipped {
			t.Fatalf("green %s class=%q boundary=%+v", intent, green.Boundary.Classification, green.Boundary)
		}
		if !green.Boundary.MediaGamutAlarm || green.Boundary.MediaInGamut == nil || *green.Boundary.MediaInGamut {
			t.Fatalf("green %s must report media gamut crossing", intent)
		}
		if green.DeltaE00 == nil || *green.DeltaE00 < 5 {
			t.Fatalf("green %s deltaE=%v want substantial error", intent, green.DeltaE00)
		}
	}

	perc := byKey["wide-green/perceptual"]
	if perc.Boundary.Classification == ClassSourceUnrepresentable {
		t.Fatal("valid BT.2020 green must not be called source-unrepresentable")
	}
	if perc.Boundary.MediaInGamut == nil || *perc.Boundary.MediaInGamut {
		t.Fatal("perceptual result must still report original media-gamut crossing")
	}
	if perc.DeltaE00 == nil {
		t.Fatal("perceptual result missing delta E")
	}

	gray := byKey["gray/relative_colorimetric"]
	if gray.Boundary.Classification != ClassOK {
		t.Fatalf("gray class = %q boundary=%+v", gray.Boundary.Classification, gray.Boundary)
	}
	if gray.DeltaE00 == nil || *gray.DeltaE00 > 2 {
		t.Fatalf("gray deltaE=%v want near zero", gray.DeltaE00)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
