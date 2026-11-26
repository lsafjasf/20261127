package rangecheck

import "testing"

func TestLargeToneRegionIsStyleCandidate(t *testing.T) {
	const w, h = 128, 128
	y := make([]byte, w*h)
	for i := range y {
		y[i] = 120
	}
	for yy := 8; yy < 120; yy++ {
		for xx := 8; xx < 120; xx++ {
			y[yy*w+xx] = 8
		}
	}
	report, err := AnalyzeFrame(Frame{Index: 0, Width: w, Height: h, Format: FormatGray8, Y: y, StrideY: w}, DefaultSettings(w, h))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Regions) == 0 {
		t.Fatal("expected a region")
	}
	if report.Regions[0].Source != SourceStylizedGrade {
		t.Fatalf("source = %q, reasons=%v", report.Regions[0].Source, report.Regions[0].Reasons)
	}
	if report.Regions[0].Verdict != VerdictIntentionalStyle {
		t.Fatalf("verdict = %q", report.Regions[0].Verdict)
	}
}

func TestThinShallowOvershootIsCodecEdge(t *testing.T) {
	const w, h = 160, 120
	y := make([]byte, w*h)
	for i := range y {
		y[i] = 220
	}
	for xx := 10; xx < 151; xx++ {
		y[60*w+xx] = 14
	}
	report, err := AnalyzeFrame(Frame{Index: 3, Width: w, Height: h, Format: FormatGray8, Y: y, StrideY: w}, DefaultSettings(w, h))
	if err != nil {
		t.Fatal(err)
	}
	var edge *Region
	for i := range report.Regions {
		if report.Regions[i].Source == SourceCodecEdge {
			edge = &report.Regions[i]
		}
	}
	if edge == nil {
		t.Fatalf("expected codec edge, got %+v", report.Regions)
	}
	if edge.Rect.Height > 3 || edge.LocalContrast < 100 {
		t.Fatalf("edge geometry/contrast wrong: %+v", edge)
	}
}
