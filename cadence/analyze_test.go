package cadence

import (
	"math"
	"testing"
)

func TestAnalyzeSegmentsMixedCadencesAndIsolatesBadSegment(t *testing.T) {
	var frames []Frame
	frames = append(frames, synthetic24p(40)...)
	frames = append(frames, synthetic60i(15)...)
	frames = append(frames, synthetic30p(40)...)

	report, err := Analyze(Media{
		Frames:     frames,
		FrameRate:  30,
		FieldOrder: HintUnknown,
	}, Options{WindowFrames: 12, MinSegmentFrames: 4})
	if err != nil {
		t.Fatal(err)
	}
	validateTestReport(t, report, len(frames))
	if len(report.Segments) < 3 {
		t.Fatalf("got %d segments, want a locally isolated bad span: %+v",
			len(report.Segments), report.Segments)
	}
	var saw24, saw30, sawUnknown bool
	for _, segment := range report.Segments {
		switch segment.Cadence {
		case Cadence24p32:
			saw24 = true
		case Cadence30p22:
			saw30 = true
		case CadenceUnknown:
			sawUnknown = true
			if segment.Recommended != "isolate_and_review" {
				t.Errorf("unknown segment action = %q", segment.Recommended)
			}
		}
	}
	if !saw24 || !saw30 {
		t.Fatalf("healthy segments were lost: %+v", report.Segments)
	}
	if !sawUnknown {
		t.Fatalf("60i-only span was not isolated: %+v", report.Segments)
	}
}

func validateTestReport(t *testing.T, report Report, frames int) {
	t.Helper()
	if len(report.Segments) == 0 {
		t.Fatal("report has no segments")
	}
	if report.Segments[0].StartFrame != 0 || report.Segments[len(report.Segments)-1].EndFrame != frames {
		t.Fatalf("segments do not cover 0..%d: %+v", frames, report.Segments)
	}
	for i, segment := range report.Segments {
		if segment.EndFrame <= segment.StartFrame {
			t.Fatalf("empty segment: %+v", segment)
		}
		if i > 0 && report.Segments[i-1].EndFrame != segment.StartFrame {
			t.Fatalf("non-contiguous segments: %+v", report.Segments)
		}
		if segment.Confidence < 0 || segment.Confidence > 1 {
			t.Fatalf("confidence outside [0,1]: %+v", segment)
		}
		if len(segment.Evidence.Basis) == 0 {
			t.Fatalf("segment has no confidence basis: %+v", segment)
		}
	}
}

func TestAnalyzeRejectsEmptyAndShortInput(t *testing.T) {
	if _, err := Analyze(Media{}, DefaultOptions()); err == nil {
		t.Fatal("empty media should fail")
	}
	if _, err := Analyze(Media{Frames: synthetic30p(2), FrameRate: 30}, DefaultOptions()); err == nil {
		t.Fatal("two-frame media should fail")
	}
	bad := synthetic30p(5)
	bad[2].Top.Pix = bad[2].Top.Pix[:10]
	if _, err := Analyze(Media{Frames: bad, FrameRate: 30}, DefaultOptions()); err == nil {
		t.Fatal("malformed field buffer should fail")
	}
}

func TestFilmPatternRejectsWrongFieldOrder(t *testing.T) {
	frames := synthetic24p(30)
	tff, bff := makeSequences(frames)
	correct := tff.window(0, len(tff.fields)).score24p(HintUnknown)
	wrong := bff.window(0, len(bff.fields)).score24p(HintUnknown)
	if correct.score <= wrong.score+0.02 {
		t.Fatalf("correct field order did not win: tff=%.3f bff=%.3f",
			correct.score, wrong.score)
	}
	if correct.periodFit < 0.75 {
		t.Fatalf("3:2 pattern fit too low: %.3f", correct.periodFit)
	}
}

func synthetic24p(count int) []Frame {
	within := [...]float64{0, 0, 1.25, 1.25, 0}
	out := make([]Frame, count)
	for i := range out {
		top := float64(i)
		bottom := top + within[i%5]
		out[i] = syntheticFrame(top, bottom)
	}
	return out
}

func synthetic30p(count int) []Frame {
	out := make([]Frame, count)
	for i := range out {
		now := float64(i)
		out[i] = syntheticFrame(now, now)
	}
	return out
}

func synthetic60i(count int) []Frame {
	out := make([]Frame, count)
	for i := range out {
		top := float64(i)
		out[i] = syntheticFrame(top, top+0.5)
	}
	return out
}

func syntheticFrame(topTime, bottomTime float64) Frame {
	return Frame{
		Top:    syntheticField(topTime),
		Bottom: syntheticField(bottomTime),
	}
}

func syntheticField(time float64) Image {
	const width, height = 32, 18
	pix := make([]uint8, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			phase := 0.45*float64(x) + 0.22*float64(y) + 0.35*time
			value := 128 + 75*math.Sin(phase) + 25*math.Cos(0.31*float64(y)-0.7*time)
			if value < 0 {
				value = 0
			}
			if value > 255 {
				value = 255
			}
			pix[y*width+x] = uint8(value)
		}
	}
	return Image{Width: width, Height: height, Pix: pix}
}
