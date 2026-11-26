package metrics

import (
	"math"
	"testing"

	"itm/internal/img"
)

func TestLuminanceRMSE(t *testing.T) {
	gt := img.New(2, 2)
	est := img.New(2, 2)
	rc := img.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}
	gt.FillRect(rc, 0.5, 0.5, 0.5)
	est.FillRect(rc, 0.7, 0.7, 0.7)
	got := LuminanceRMSE(gt, est, rc)
	if math.Abs(got-0.2) > 1e-12 {
		t.Fatalf("RMSE=%v want 0.2", got)
	}
}

func TestHueErrorNeutralOnGray(t *testing.T) {
	gt := img.New(2, 2)
	est := img.New(2, 2)
	rc := img.Rect{X0: 0, Y0: 0, X1: 2, Y1: 2}
	gt.FillRect(rc, 0.4, 0.4, 0.4)
	est.FillRect(rc, 0.8, 0.8, 0.8)
	deg, _, n := HueError(gt, est, rc)
	if n != 0 || deg != 0 {
		t.Fatalf("gray regions have no hue, got n=%d deg=%v", n, deg)
	}
}

func TestNoiseAmplification(t *testing.T) {
	gt := img.New(1, 4)
	est := img.New(1, 4)
	rc := img.Rect{X0: 0, Y0: 0, X1: 4, Y1: 1}
	// 真值亮度 0.1,0.2,0.3,0.4 -> std；估计放大 2 倍。
	gtVals := []float64{0.1, 0.2, 0.3, 0.4}
	for x, v := range gtVals {
		gt.Set(x, 0, v, v, v)
		est.Set(x, 0, 2*v, 2*v, 2*v)
	}
	_, _, ratio := NoiseAmplification(gt, est, rc)
	if math.Abs(ratio-2.0) > 1e-9 {
		t.Fatalf("noise ratio=%v want 2", ratio)
	}
}
