package iccproof

import (
	"strings"
	"testing"
)

func TestParseRequestJSONRejectsUnknownFieldsAndKeepsProfilesExplicit(t *testing.T) {
	_, err := ParseRequestJSON(strings.NewReader(`{"source_profile":{"path":"a.icc"},"media_profile":{"path":"b.icc"},"bogus":1}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err=%v want unknown field", err)
	}
	req, err := ParseRequestJSON(strings.NewReader(`{"source_profile":{"path":"a.icc"},"media_profile":{"path":"b.icc"},"samples":[{"rgb":[0,1,0]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Intents) != 3 {
		t.Fatalf("default intents = %v", req.Intents)
	}
}
