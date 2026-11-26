package contract

import "testing"

func TestLoadRejectsBadSchema(t *testing.T) {
	if _, err := Load([]byte(`{"schemaVersion":"9.9","platform":"p","contractId":"c"}`)); err == nil {
		t.Fatal("expected schema version error")
	}
}

func TestLoadRejectsInvalidRangeAndRegex(t *testing.T) {
	cases := []string{
		`{"schemaVersion":"1.0","platform":"p","contractId":"c",
		 "video":{"bitrate":{"minBps":10,"maxBps":5}}}`,
		`{"schemaVersion":"1.0","platform":"p","contractId":"c",
		 "video":{"frameRate":{"allowed":[{"num":1,"den":0}]}}}`,
		`{"schemaVersion":"1.0","platform":"p","contractId":"c",
		 "filename":{"regex":"["}}`,
		`{"schemaVersion":"1.0","platform":"p","contractId":"c",
		 "audio":{"minStreams":3,"maxStreams":2}}`,
	}
	for i, data := range cases {
		if _, err := Load([]byte(data)); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestLoadAcceptsMinimalContract(t *testing.T) {
	c, err := Load([]byte(`{"schemaVersion":"1.0","platform":"p","contractId":"c"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SchemaVersion != SchemaVersion {
		t.Fatalf("schema = %q", c.SchemaVersion)
	}
}
