package media

import (
	"testing"

	"colorfix/internal/color"
)

func TestMatrixFilter(t *testing.T) {
	tests := []struct {
		codec  string
		matrix color.Matrix
		want   string
	}{
		{"h264", color.BT601, "h264_metadata=matrix_coefficients=5"},
		{"h264", color.BT709, "h264_metadata=matrix_coefficients=1"},
		{"hevc", color.BT601, "hevc_metadata=matrix_coefficients=5"},
		{"mpeg2video", color.BT709, "mpeg2_metadata=matrix_coefficients=1"},
		{"av1", color.BT601, "av1_metadata=matrix_coefficients=5"},
		{"vp9", color.BT601, "vp9_metadata=color_space=1"},
		{"vp9", color.BT709, "vp9_metadata=color_space=2"},
	}
	for _, test := range tests {
		got, ok := matrixFilter(test.codec, test.matrix)
		if !ok || got != test.want {
			t.Errorf("matrixFilter(%s, %s) = %q, %t; want %q", test.codec, test.matrix, got, ok, test.want)
		}
	}
}
