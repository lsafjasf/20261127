package main

import "testing"

func TestParseROI(t *testing.T) {
	roi, err := parseROI("10,20,300,120")
	if err != nil {
		t.Fatal(err)
	}
	if roi.X != 10 || roi.Y != 20 || roi.W != 300 || roi.H != 120 {
		t.Fatalf("unexpected ROI: %+v", roi)
	}
	if _, err := parseROI("10,20,0,120"); err == nil {
		t.Fatal("expected zero-width ROI to fail")
	}
	if _, err := parseROI("10,20,30"); err == nil {
		t.Fatal("expected incomplete ROI to fail")
	}
}

func TestParseBlockSizes(t *testing.T) {
	sizes, err := parseBlockSizes("4,8,16")
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 3 || sizes[0] != 4 || sizes[2] != 16 {
		t.Fatalf("unexpected sizes: %v", sizes)
	}
	if _, err := parseBlockSizes("8,abc"); err == nil {
		t.Fatal("expected invalid size to fail")
	}
}
