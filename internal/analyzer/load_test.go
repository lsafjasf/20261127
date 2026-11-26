package analyzer

import (
	"path/filepath"
	"testing"
)

func TestNaturalSort(t *testing.T) {
	paths := []string{
		filepath.Join("frames", "frame_10.png"),
		filepath.Join("frames", "frame_2.png"),
		filepath.Join("frames", "frame_1.png"),
	}
	naturalSort(paths)
	want := []string{"frame_1.png", "frame_2.png", "frame_10.png"}
	for i, path := range paths {
		if filepath.Base(path) != want[i] {
			t.Fatalf("order = %v, want %v", paths, want)
		}
	}
}

func TestMapROI(t *testing.T) {
	roi, err := mapROI(Rect{X: 100, Y: 50, W: 200, H: 80}, 1.5, 1.5, 900, 600)
	if err != nil {
		t.Fatal(err)
	}
	if roi != (Rect{X: 150, Y: 75, W: 300, H: 120}) {
		t.Fatalf("mapped ROI = %+v", roi)
	}
}
