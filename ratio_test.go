package aperturemap

import (
	"encoding/json"
	"testing"
)

func TestParseRatioForms(t *testing.T) {
	cases := []struct {
		text string
		want Ratio
	}{
		{"16:9", MustRatio(16, 9)},
		{"16/9", MustRatio(16, 9)},
		{"1.5", MustRatio(3, 2)},
		{"1", MustRatio(1, 1)},
	}
	for _, tc := range cases {
		got, err := ParseRatio(tc.text)
		if err != nil {
			t.Fatalf("ParseRatio(%q): %v", tc.text, err)
		}
		if !got.Equal(tc.want) {
			t.Fatalf("ParseRatio(%q) = %s, want %s", tc.text, got, tc.want)
		}
	}
}

func TestRatioJSONRoundTrip(t *testing.T) {
	type wrapper struct {
		PAR Ratio `json:"par"`
	}

	var decoded wrapper
	if err := json.Unmarshal([]byte(`{"par":"64:45"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.PAR.Equal(MustRatio(64, 45)) {
		t.Fatalf("PAR = %s", decoded.PAR)
	}

	out, err := json.Marshal(wrapper{PAR: MustRatio(64, 45)})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"par":"64/45"}` {
		t.Fatalf("json = %s", out)
	}
}

func TestRatioObjectJSON(t *testing.T) {
	var r Ratio
	if err := json.Unmarshal([]byte(`{"n":4,"d":3}`), &r); err != nil {
		t.Fatal(err)
	}
	if !r.Equal(MustRatio(4, 3)) {
		t.Fatalf("ratio = %s", r)
	}
}
