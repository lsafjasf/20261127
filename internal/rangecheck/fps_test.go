package rangecheck

import "testing"

func TestVariableFrameIntervalsAreFlagged(t *testing.T) {
	const w, h = 16, 16
	settings := DefaultSettings(w, h)
	analyzer := NewAnalyzer("vfr", settings, Metadata{})
	for i := 0; i < 12; i++ {
		pts := float64(i) * 0.04
		if i%2 == 0 {
			pts -= 0.01
		} else {
			pts += 0.01
		}
		y := make([]byte, w*h)
		for j := range y {
			y[j] = 128
		}
		if _, err := analyzer.Add(Frame{Index: i, PTSSeconds: pts, Width: w, Height: h, Format: FormatGray8, Y: y, StrideY: w}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := analyzer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if report.FPS.VFRScore < 0.3 {
		t.Fatalf("VFR score = %.2f, want high jitter score: %+v", report.FPS.VFRScore, report.FPS)
	}
	if len(report.FPS.Warnings) == 0 {
		t.Fatal("expected VFR warning")
	}
}
