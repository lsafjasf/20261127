package bundle

import "testing"

func TestLanguageFromName(t *testing.T) {
	cases := map[string]string{
		"movie.en-US.srt":   "en-US",
		"movie_pt-BR.aac":   "pt-BR",
		"track.zh.alt.flac": "zh",
		"/tmp/x/ja.opus":    "ja",
		"random-name.mp3":   Undetermined,
	}
	for name, want := range cases {
		if got := LanguageFromName(name); got != want {
			t.Errorf("LanguageFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestEnsureUniquePaths(t *testing.T) {
	tracks := []Track{
		{RelPath: "audio/en/a.m4a"},
		{RelPath: "audio/en/a.m4a"},
		{RelPath: "audio/en/a.m4a"},
		{RelPath: "audio/en/b.m4a"},
	}
	ensureUniquePaths(tracks)
	seen := map[string]bool{}
	for _, tr := range tracks {
		if seen[tr.RelPath] {
			t.Fatalf("路径仍重复: %s", tr.RelPath)
		}
		seen[tr.RelPath] = true
	}
	if tracks[1].RelPath != "audio/en/a~2.m4a" || tracks[2].RelPath != "audio/en/a~3.m4a" {
		t.Fatalf("去重命名错误: %s, %s", tracks[1].RelPath, tracks[2].RelPath)
	}
}
