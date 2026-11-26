package observermap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssessMissingProfileIsUncertain(t *testing.T) {
	c := AssessProfile("")
	if !c.Uncertain || c.ProfilePresent {
		t.Errorf("empty profile must be uncertain/absent: %+v", c)
	}
	if c.ObserverRecorded {
		t.Error("ICC observer must be recorded as absent")
	}

	c2 := AssessProfile(filepath.Join(t.TempDir(), "nope.icc"))
	if !c2.Uncertain || c2.ProfileError == "" {
		t.Error("unreadable profile path must be uncertain with error")
	}
}

func TestParseFixtures(t *testing.T) {
	cases := []struct {
		file      string
		white     string
		uncertain bool
	}{
		{"monitor-d50.icc", "D50", false},
		{"monitor-d65.icc", "D65", false},
		{"illuminant-a-white.icc", "UNSUPPORTED", true},
	}
	for _, c := range cases {
		path := filepath.Join("testdata", c.file)
		cert := AssessProfile(path)
		if cert.ProfileError != "" {
			t.Fatalf("%s: parse error: %s", c.file, cert.ProfileError)
		}
		if cert.WhitePoint != c.white {
			t.Errorf("%s: white=%s want %s", c.file, cert.WhitePoint, c.white)
		}
		if cert.Uncertain != c.uncertain {
			t.Errorf("%s: uncertain=%v want %v", c.file, cert.Uncertain, c.uncertain)
		}
		if cert.Profile == nil || cert.Profile.MediaWhitePoint == nil {
			t.Errorf("%s: profile/wtpt not parsed", c.file)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	good, err := os.ReadFile(filepath.Join("testdata", "monitor-d50.icc"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseProfileBytes(good[:64]); err == nil {
		t.Error("truncated profile should fail")
	}
	bad := append([]byte(nil), good...)
	copy(bad[36:40], "xxxx")
	if _, err := ParseProfileBytes(bad); err == nil {
		t.Error("bad acsp magic should fail")
	}
}

func TestRealGhostscriptSRGBProfileIfPresent(t *testing.T) {
	const p = "/usr/share/color/icc/ghostscript/srgb.icc"
	if _, err := os.Stat(p); err != nil {
		t.Skip("system srgb.icc not present")
	}
	cert := AssessProfile(p)
	if cert.ProfileError != "" {
		t.Fatalf("real profile parse: %s", cert.ProfileError)
	}
	if cert.WhitePoint != "D65" || cert.Uncertain {
		t.Errorf("ghostscript sRGB expected supported D65, got %s uncertain=%v", cert.WhitePoint, cert.Uncertain)
	}
}
