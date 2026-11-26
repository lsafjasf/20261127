package aperturemap

import (
	"errors"
	"testing"
)

func TestCanonicalCombinations(t *testing.T) {
	for _, combo := range CanonicalCombinations() {
		t.Run(combo.ID, func(t *testing.T) {
			got, err := Resolve(combo.Spec)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if !got.DisplayAspectRatio.Equal(MustRatio(16, 9)) {
				t.Fatalf("DAR = %s, want 16/9", got.DisplayAspectRatio)
			}
			if got.Transform.ScaleX.Cmp(MustRatio(0, 1)) == 0 || got.Transform.ScaleY.Cmp(MustRatio(0, 1)) == 0 {
				t.Fatalf("invalid zero scale: %+v", got.Transform)
			}
			if len(got.Mappings) != 3 {
				t.Fatalf("mapping stages = %d, want 3", len(got.Mappings))
			}
			composed := got.Mappings[0].Transform.Then(got.Mappings[1].Transform).Then(got.Mappings[2].Transform)
			assertRatio(t, composed.ScaleX, got.Transform.ScaleX, "composed scale x")
			assertRatio(t, composed.ScaleY, got.Transform.ScaleY, "composed scale y")
			assertRatio(t, composed.TranslateX, got.Transform.TranslateX, "composed tx")
			assertRatio(t, composed.TranslateY, got.Transform.TranslateY, "composed ty")
		})
	}
}

func TestCanonicalTransforms(t *testing.T) {
	viewport := Size{Width: 1920, Height: 1080}

	direct, err := Resolve(Spec{
		EffectivePixels: Size{3840, 2160},
		Viewport:        &viewport,
		Fit:             FitExact,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRatio(t, direct.Transform.ScaleX, MustRatio(1, 2), "direct scale x")
	assertRatio(t, direct.Transform.ScaleY, MustRatio(1, 2), "direct scale y")

	anamorphic, err := Resolve(Spec{
		EffectivePixels:  Size{1440, 1080},
		PixelAspectRatio: MustRatio(4, 3),
		Viewport:         &viewport,
		Fit:              FitExact,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRatio(t, anamorphic.Transform.ScaleX, MustRatio(4, 3), "PAR scale x")
	assertRatio(t, anamorphic.Transform.ScaleY, MustRatio(1, 1), "PAR scale y")

	clapped, err := Resolve(Spec{
		EffectivePixels: Size{1920, 1088},
		CleanAperture: &CleanAperture{
			Width:          MustRatio(1920, 1),
			Height:         MustRatio(1080, 1),
			VerticalOffset: MustRatio(-4, 1),
		},
		Viewport: &viewport,
		Fit:      FitExact,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRatio(t, clapped.CleanAperture.ExactRect.Min.Y, MustRatio(-1, 2), "clap min y")
	assertRatio(t, clapped.CleanAperture.ExactRect.Max.Y, MustRatio(2159, 2), "clap max y") // 1079.5
	assertRatio(t, clapped.Transform.TranslateY, MustRatio(1, 2), "clap translate y")
}

func TestRejectionExamples(t *testing.T) {
	for _, combo := range RejectionExamples() {
		t.Run(combo.ID, func(t *testing.T) {
			_, err := Resolve(combo.Spec)
			if err == nil {
				t.Fatal("Resolve() succeeded, want rejection")
			}
		})
	}
}

func TestDeclaredDARConflictRejected(t *testing.T) {
	badDAR := MustRatio(4, 3)
	_, err := Resolve(Spec{
		EffectivePixels:    Size{1920, 1080},
		DisplayAspectRatio: &badDAR,
	})
	if !errors.Is(err, ErrInconsistentRatio) {
		t.Fatalf("error = %v, want ErrInconsistentRatio", err)
	}
}

func TestClapOutOfBoundsRejected(t *testing.T) {
	_, err := Resolve(Spec{
		EffectivePixels: Size{1920, 1080},
		CleanAperture: &CleanAperture{
			Width:            MustRatio(1920, 1),
			Height:           MustRatio(1080, 1),
			HorizontalOffset: MustRatio(1, 1),
		},
	})
	if !errors.Is(err, ErrApertureOutOfBounds) {
		t.Fatalf("error = %v, want ErrApertureOutOfBounds", err)
	}
}

func TestExactFitRejectsPlayerAspectMismatch(t *testing.T) {
	viewport := Size{Width: 2000, Height: 1000} // 2:1, wider than 16:9
	_, err := Resolve(Spec{
		EffectivePixels: Size{1920, 1080},
		Viewport:        &viewport,
		Fit:             FitExact,
	})
	if !errors.Is(err, ErrPresentationImpossible) {
		t.Fatalf("error = %v, want ErrPresentationImpossible", err)
	}
}

func TestContainReportsLetterboxFraction(t *testing.T) {
	wide := Size{Width: 2000, Height: 1000}
	contained, err := Resolve(Spec{
		EffectivePixels: Size{1920, 1080},
		Viewport:        &wide,
		Fit:             FitContain,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contained.Diagnosis.Letterboxed {
		t.Fatal("contain into wider viewport should letterbox")
	}
	assertRatio(t, contained.Diagnosis.HorizontalBarFraction, MustRatio(1, 9), "horizontal bar fraction")
	assertRatio(t, contained.Diagnosis.VerticalBarFraction, MustRatio(0, 1), "vertical bar fraction")
}

func TestUnknownFitRejectedWithoutViewport(t *testing.T) {
	_, err := Resolve(Spec{
		EffectivePixels: Size{1920, 1080},
		Fit:             FitMode("pan"),
	})
	if !errors.Is(err, ErrInvalidGeometry) {
		t.Fatalf("error = %v, want ErrInvalidGeometry", err)
	}
}

func TestStretchAndCoverMappings(t *testing.T) {
	wide := Size{Width: 2000, Height: 1000} // 2:1 viewport

	stretched, err := Resolve(Spec{
		EffectivePixels: Size{1920, 1080},
		Viewport:        &wide,
		Fit:             FitStretch,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRatio(t, stretched.TargetRect.Max.X, MustRatio(2000, 1), "stretch target width")
	assertRatio(t, stretched.Transform.ScaleX, MustRatio(25, 24), "stretch x scale") // 2000/1920
	assertRatio(t, stretched.Transform.ScaleY, MustRatio(25, 27), "stretch y scale") // 1000/1080
	if stretched.Transform.ScaleX.Cmp(stretched.Transform.ScaleY) <= 0 {
		t.Fatal("wide viewport stretch should enlarge x more than y")
	}
	assertRatio(t, stretched.Diagnosis.PhysicalStretch, MustRatio(9, 8), "physical stretch")
	if stretched.Diagnosis.UniformPhysicalScale {
		t.Fatal("stretched mapping must not be diagnosed uniform")
	}

	covered, err := Resolve(Spec{
		EffectivePixels: Size{1920, 1080},
		Viewport:        &wide,
		Fit:             FitCover,
	})
	if err != nil {
		t.Fatal(err)
	}
	if covered.VisibleSourceRect == nil {
		t.Fatal("cover mapping must include visible source rect")
	}
	assertRatio(t, covered.VisibleSourceRect.Height(), MustRatio(960, 1), "cover visible height")
	assertRatio(t, covered.VisibleSourceRect.Width(), MustRatio(1920, 1), "cover visible width")
	if !covered.Diagnosis.PlayerCropped {
		t.Fatal("cover mapping should diagnose player crop")
	}
	assertRatio(t, covered.Diagnosis.CropHeightFraction, MustRatio(1, 9), "cover crop fraction")
}

func TestFractionalClapPreservedExactly(t *testing.T) {
	got, err := Resolve(Spec{
		EffectivePixels: Size{Width: 100, Height: 100},
		CleanAperture: &CleanAperture{
			Width:            MustRatio(99, 2), // 49.5
			Height:           MustRatio(99, 2),
			HorizontalOffset: MustRatio(1, 4),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ca := got.CleanAperture
	if ca == nil {
		t.Fatal("clean aperture not resolved")
	}
	// center = 99/2 + 1/4 = 199/4; half width = 99/4.
	assertRatio(t, ca.ExactRect.Min.X, MustRatio(25, 1), "fractional min x")
	assertRatio(t, ca.ExactRect.Max.X, MustRatio(149, 2), "fractional max x")
}

func assertRatio(t *testing.T, got, want Ratio, context string) {
	t.Helper()
	if !got.Equal(want) {
		t.Fatalf("%s = %s, want %s", context, got, want)
	}
}
