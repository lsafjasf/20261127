package fuzzyx

import "testing"

func TestIdenticalIsMaxSimilarityButNotIdentity(t *testing.T) {
	a := []byte("the quick brown fox jumps over the lazy dog and keeps running")
	r := Compare(a, a)
	if r.JaccardPercent != 100 {
		t.Fatalf("identical jaccard=%v, want 100", r.JaccardPercent)
	}
	if r.HammingBits != 0 {
		t.Fatalf("identical hamming=%d, want 0", r.HammingBits)
	}
	// A zero Hamming distance is still only a fuzzy signal: the package does
	// not expose any API that turns it into a source/identity conclusion.
	if !r.IsLead() {
		t.Fatal("identical objects should clear the lead threshold")
	}
}

func TestSmallEditStaysSimilarUnrelatedScoresLow(t *testing.T) {
	base := []byte("值班日志：凌晨两点巡逻三号仓库，封条完好，一切正常，记录在案。")
	edit := []byte("值班日志：凌晨一点巡逻五号仓库，封条完好，一切正常，记录在案。")
	r := Compare(base, edit)
	if r.JaccardPercent < 50 {
		t.Fatalf("small same-length edit should stay highly similar, got %.1f", r.JaccardPercent)
	}
	unrelated := []byte("XYZQWERTY9876543210-alpha-beta-gamma-delta-epsilon")
	r2 := Compare(base, unrelated)
	if r2.JaccardPercent > 10 {
		t.Fatalf("unrelated data should score very low, got %.1f", r2.JaccardPercent)
	}
}

func TestParseSimHash(t *testing.T) {
	d := Sum([]byte("some fixture content for simhash"))
	v, err := ParseSimHash(d.Algorithm + ":v1:" + d.SimHash)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ParseSimHash(d.SimHash)
	if err != nil {
		t.Fatal(err)
	}
	if v != want {
		t.Fatal("prefixed digest must parse to same simhash")
	}
}
