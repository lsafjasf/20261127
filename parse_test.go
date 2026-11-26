package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseCSVAliasesAndMS(t *testing.T) {
	csv := "frame_no,time_ms,phash,dropped\n" +
		"1,1764000000000,0000000000000001,false\n" +
		"2,1764000000033,0000000000000003,0\n" +
		"3,1764000000066,,true\n"
	p := writeTemp(t, "frames.csv", csv)
	tr, err := LoadTraceFile(p, StageRecord)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Frames) != 2 || len(tr.Markers) != 1 {
		t.Fatalf("frames=%d markers=%d, want 2/1", len(tr.Frames), len(tr.Markers))
	}
	f0 := tr.Frames[0]
	if !f0.HasSeq || f0.Seq != 1 {
		t.Fatalf("seq = %d", f0.Seq)
	}
	if !f0.HasPTS || absF(f0.PTS-1764000000.0) > 1 {
		t.Fatalf("pts = %v, want unix seconds ~1764000000", f0.PTS)
	}
	if !f0.Sig.Valid() || f0.Sig.Kind != SigPHash64 || f0.Sig.P != 1 {
		t.Fatalf("sig = %+v", f0.Sig)
	}
	if tr.Markers[0].Seq != 3 {
		t.Fatalf("marker seq = %d, want 3", tr.Markers[0].Seq)
	}
}

func TestParseJSONLAdvance(t *testing.T) {
	jsonl := `{"seq":7,"pts_time":1.5,"dhash":"000000000000000f","diff":0.2}` + "\n" +
		`{"frame":8,"pts":1.533,"md5":"0123456789abcdef0123456789abcdef"}` + "\n"
	p := writeTemp(t, "f.jsonl", jsonl)
	tr, err := LoadTraceFile(p, StageRecord)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Frames) != 2 {
		t.Fatalf("frames=%d", len(tr.Frames))
	}
	if !tr.Frames[0].HasAdvance || absF(tr.Frames[0].Advance-0.2) > 1e-9 {
		t.Fatalf("advance = %v", tr.Frames[0].Advance)
	}
	if tr.Frames[1].Sig.Kind != SigExact {
		t.Fatalf("md5 sig kind = %d, want exact", tr.Frames[1].Sig.Kind)
	}
}

func TestParseSigKinds(t *testing.T) {
	s, err := ParseSig("phash:000000000000000f")
	if err != nil || s.Kind != SigPHash64 || s.P != 15 {
		t.Fatalf("phash parse: %+v err=%v", s, err)
	}
	a, _ := ParseSig("0000000000000000")
	b, _ := ParseSig("0000000000000007")
	if d := SigDistance(a, b); absF(d-3.0/64.0) > 1e-12 {
		t.Fatalf("distance = %v", d)
	}
	if !SigSame(a, b, 3) || SigSame(a, b, 2) {
		t.Fatal("SigSame tolerance wrong")
	}
}

func TestInferStage(t *testing.T) {
	cases := map[string]Stage{
		"capture.csv":  StageCapture,
		"cam_log.json": StageCapture,
		"record.mp4":   StageRecord,
		"player.log":   StagePlayer,
		"render.csv":   StagePlayer,
		"weird.csv":    StageUnknown,
	}
	for name, want := range cases {
		if got := InferStage(name); got != want {
			t.Errorf("InferStage(%q)=%s want %s", name, got, want)
		}
	}
}
