package rangecheck

import (
	"math"
	"strings"
	"testing"
)

func TestSyntheticSubtitleAndHighlightAreLocalized(t *testing.T) {
	const w, h = 320, 180
	settings := DefaultSettings(w, h)
	settings.FPS = Rational{Num: 30, Den: 1}

	analyzer := NewAnalyzer("synthetic", settings, Metadata{})
	for frameIndex := 0; frameIndex < 12; frameIndex++ {
		y := make([]byte, w*h)
		for i := range y {
			y[i] = 112
		}
		for glyph := 0; glyph < 8; glyph++ {
			x0 := 38 + glyph*20
			for yy := 148; yy < 170; yy++ {
				for xx := x0; xx < x0+12; xx++ {
					y[yy*w+xx] = 0
				}
			}
		}
		for yy := 34; yy < 76; yy++ {
			for xx := 190; xx < 252; xx++ {
				y[yy*w+xx] = 255
			}
		}
		frame := Frame{Index: frameIndex, PTSSeconds: float64(frameIndex) / 30, Width: w, Height: h, Format: FormatGray8, Y: y, StrideY: w}
		report, err := analyzer.Add(frame)
		if err != nil {
			t.Fatalf("Add frame %d: %v", frameIndex, err)
		}
		if !report.HasOutOfRange {
			t.Fatalf("frame %d should contain out-of-range pixels", frameIndex)
		}
		if report.Y.ExactBlack == 0 || report.Y.ExactWhite == 0 {
			t.Fatalf("frame %d missing anchors: black=%d white=%d", frameIndex, report.Y.ExactBlack, report.Y.ExactWhite)
		}
		if len(report.Regions) < 2 {
			t.Fatalf("frame %d regions = %d, want at least black and white", frameIndex, len(report.Regions))
		}
	}

	report, err := analyzer.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(report.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(report.Segments))
	}
	seg := report.Segments[0]
	if seg.FirstFrame != 0 || seg.LastFrame != 11 || seg.FrameCount != 12 {
		t.Fatalf("bad segment span: %+v", seg)
	}
	if !strings.HasPrefix(seg.Classification, "localized_superblack_subtitle_and_superwhite_highlight") {
		t.Fatalf("classification = %q", seg.Classification)
	}

	var foundSubtitle, foundHighlight bool
	for _, track := range report.Tracks {
		if track.Source == SourceBurnedSubtitle {
			foundSubtitle = true
			if track.Representative.Y < int(float64(h)*0.65) {
				t.Fatalf("subtitle track not in lower area: %+v", track.Representative)
			}
			if track.FrameCount < 8 || track.BestConfidence < 0.8 {
				t.Fatalf("subtitle temporal confidence too weak: %+v", track)
			}
		}
		if track.Source == SourceHighlight {
			foundHighlight = true
			if track.Representative.X < w/2 {
				t.Fatalf("highlight box unexpectedly on left: %+v", track.Representative)
			}
		}
	}
	if !foundSubtitle || !foundHighlight {
		t.Fatalf("sources missing subtitle=%v highlight=%v tracks=%+v", foundSubtitle, foundHighlight, report.Tracks)
	}
	if math.Abs(report.FPS.DeclaredFPS-30) > 0.001 || report.FPS.VFRScore != 0 {
		t.Fatalf("bad FPS report: %+v", report.FPS)
	}
}

func TestLegalCrushWallsAreNotOutOfRange(t *testing.T) {
	const w, h = 64, 64
	settings := DefaultSettings(w, h)
	analyzer := NewAnalyzer("legal", settings, Metadata{})
	y := make([]byte, w*h)
	for i := range y {
		if i%2 == 0 {
			y[i] = 16
		} else {
			y[i] = 235
		}
	}
	frameReport, err := analyzer.Add(Frame{Index: 0, Width: w, Height: h, Format: FormatGray8, Y: y, StrideY: w})
	if err != nil {
		t.Fatal(err)
	}
	if frameReport.HasOutOfRange {
		t.Fatalf("16 and 235 must be legal in TV range: %+v", frameReport.Y)
	}
	report, err := analyzer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Segments) != 0 {
		t.Fatalf("legal levels created segments: %+v", report.Segments)
	}
}
