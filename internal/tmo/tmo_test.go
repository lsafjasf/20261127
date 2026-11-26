package tmo

import (
	"math"
	"testing"

	"itm/internal/img"
)

const (
	testN     = 1.2
	testWhite = 1.0
	testUpper = 8.0
)

func TestForwardMonotonic(t *testing.T) {
	prev := -1.0
	for x := 0.0; x <= testWhite; x += 0.05 {
		y := Forward(x, testN, testWhite)
		if y < prev {
			t.Fatalf("Forward not monotonic at x=%v", x)
		}
		prev = y
	}
}

func TestSoftShoulderInvertsForward(t *testing.T) {
	for _, x := range []float64{0.02, 0.1, 0.3, 0.7, 0.95} {
		y := Forward(x, testN, testWhite)
		got := invertForward(y, testN, testWhite, testUpper)
		if math.Abs(got-x) > 1e-6 {
			t.Fatalf("invertForward(Forward(%v))=%v", x, got)
		}
	}
}

func TestSoftShoulderImageRoundTrip(t *testing.T) {
	m := img.New(4, 4)
	for i := range m.R {
		m.R[i], m.G[i], m.B[i] = 0.42, 0.27, 0.61
	}
	sdr := CameraToSDR(m, testN, testWhite)
	back := SoftShoulder(sdr, testN, testWhite, testUpper)
	for i := range m.R {
		if math.Abs(back.R[i]-m.R[i]) > 0.02 {
			t.Fatalf("channel R round trip %v -> %v", m.R[i], back.R[i])
		}
	}
}

func TestClippedSDRMapsToUpper(t *testing.T) {
	if got := invertForward(1.0, testN, testWhite, testUpper); got != testUpper {
		t.Fatalf("clipped SDR should map to upper, got %v", got)
	}
}

func TestLocalProtectDistinguishesFlatAndSpot(t *testing.T) {
	// 左半：亮而平坦的“天空”；右半暗背景中心放一个小镜面亮点。
	m := img.New(64, 64)
	m.FillRect(img.Rect{X0: 0, Y0: 0, X1: 32, Y1: 64}, 0.62, 0.7, 0.9)
	m.FillRect(img.Rect{X0: 32, Y0: 0, X1: 64, Y1: 64}, 0.05, 0.05, 0.05)
	m.FillRect(img.Rect{X0: 44, Y0: 28, X1: 52, Y1: 36}, 1.0, 1.0, 1.0)

	sdr := CameraToSDR(m, testN, testWhite)
	out := LocalProtect(sdr, testN, testWhite, testUpper, 8, 0.2, 0.6, 0.35)

	skyGain := out.R[16] / sdr.R[16]
	spotIdx := 32*64 + 44
	spotGain := out.R[spotIdx] / sdr.R[spotIdx]
	if skyGain > 1.15 {
		t.Fatalf("flat bright sky must be damped, gain=%v", skyGain)
	}
	if spotGain < 2.0 {
		t.Fatalf("local specular spot must keep expansion gain, got %v", spotGain)
	}
}

func TestLocalProtectPreservesHue(t *testing.T) {
	m := img.New(16, 16)
	m.FillRect(img.Rect{X0: 0, Y0: 0, X1: 16, Y1: 16}, 0.5, 0.3, 0.2)
	sdr := CameraToSDR(m, testN, testWhite)
	out := LocalProtect(sdr, testN, testWhite, testUpper, 4, 0.2, 0.6, 0.35)
	ratio := out.R[0] / sdr.R[0]
	for i := range sdr.R {
		for ch, pair := range [][2]float64{
			{out.R[i], sdr.R[i]}, {out.G[i], sdr.G[i]}, {out.B[i], sdr.B[i]},
		} {
			if math.Abs(pair[0]/pair[1]-ratio) > 1e-12 {
				t.Fatalf("channel %d uses different gain, hue not preserved", ch)
			}
		}
	}
}
