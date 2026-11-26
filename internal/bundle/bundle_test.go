package bundle

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "audio", "movie.en.m4a"), "en-audio")
	writeFile(t, filepath.Join(root, "audio", "movie.ja.mp3"), "ja-audio")
	writeFile(t, filepath.Join(root, "subs", "movie.en.srt"), "en-sub")
	writeFile(t, filepath.Join(root, "subs", "movie.en.ass"), "en-sub-ass")
	writeFile(t, filepath.Join(root, "chapters", "movie.en.xml"), "en-ch")
	writeFile(t, filepath.Join(root, "covers", "poster.en.png"), "en-cover")
	writeFile(t, filepath.Join(root, "covers", "poster.ja.jpg"), "ja-cover")
	return root
}

func baseOptions(root, out string) Options {
	return Options{
		VideoID:               "vid-1",
		AudioDir:              filepath.Join(root, "audio"),
		SubtitleDir:           filepath.Join(root, "subs"),
		ChapterDir:            filepath.Join(root, "chapters"),
		CoverDir:              filepath.Join(root, "covers"),
		Out:                   out,
		DuplicatePolicy:       DuplicatePrefer,
		MissingSubtitlePolicy: MissingSubtitleWarn,
	}
}

func TestBuildSuccessAndPortablePaths(t *testing.T) {
	root := fixtureTree(t)
	out := filepath.Join(t.TempDir(), "delivery")

	res, err := Build(baseOptions(root, out))
	if err != nil {
		t.Fatalf("Build 失败: %v", err)
	}

	if _, err := os.Stat(filepath.Join(out, manifestName)); err != nil {
		t.Fatalf("清单缺失: %v", err)
	}
	for _, f := range res.Manifest.Files {
		if filepath.IsAbs(f.Path) || strings.Contains(f.Path, "..") ||
			strings.ContainsRune(f.Path, '\\') {
			t.Fatalf("清单中出现不可移植路径: %q", f.Path)
		}
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f.Path))); err != nil {
			t.Fatalf("清单文件未落盘 %s: %v", f.Path, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(out, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	blob := string(data)
	if strings.Contains(blob, root) {
		t.Fatalf("清单泄露了源机器绝对路径: %s", root)
	}

	enCell := res.Manifest.Matrix.Cells[KindSubtitle]["en"]
	if !enCell.Present || enCell.Chosen != "subtitles/en/movie.en.ass" {
		t.Fatalf("重复字幕裁决错误: %+v", enCell)
	}
	if _, err := os.Stat(filepath.Join(out, "subtitles", "en", "movie.en.srt")); !os.IsNotExist(err) {
		t.Fatalf("未入选的重复字幕不应进入交付包")
	}

	foundWarn := false
	for _, w := range res.Manifest.Warnings {
		if strings.Contains(w, `语言 "ja" 缺少对应字幕`) {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("缺少 ja 字幕的警告未生成: %v", res.Manifest.Warnings)
	}
}

func TestDuplicateErrorPolicy(t *testing.T) {
	root := fixtureTree(t)
	out := filepath.Join(t.TempDir(), "delivery")
	opts := baseOptions(root, out)
	opts.DuplicatePolicy = DuplicateError

	_, err := Build(opts)
	if err == nil || !strings.Contains(err.Error(), "重复语言") {
		t.Fatalf("期望重复语言错误，得到: %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("失败后输出目录必须不存在")
	}
}

func TestMissingSubtitleFailPolicy(t *testing.T) {
	root := fixtureTree(t)
	out := filepath.Join(t.TempDir(), "delivery")
	opts := baseOptions(root, out)
	opts.MissingSubtitlePolicy = MissingSubtitleFail

	_, err := Build(opts)
	if err == nil || !strings.Contains(err.Error(), "缺少对应字幕") {
		t.Fatalf("期望缺失字幕错误，得到: %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("失败后输出目录必须不存在")
	}
}

func TestRollbackOnCopyFailure(t *testing.T) {
	root := fixtureTree(t)
	// 删除一个源文件的读权限，强制复制阶段失败。
	target := filepath.Join(root, "audio", "movie.ja.mp3")
	if err := os.Chmod(target, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o644) })

	parent := t.TempDir()
	out := filepath.Join(parent, "delivery")
	_, err := Build(baseOptions(root, out))
	if err == nil {
		t.Fatal("期望复制失败触发回滚")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("输出目录残留: %s", out)
	}
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".videopack-") {
			t.Fatalf("暂存目录残留: %s", e.Name())
		}
	}
}

func TestExistingOutRejectedAndForceReplace(t *testing.T) {
	root := fixtureTree(t)
	out := filepath.Join(t.TempDir(), "delivery")
	opts := baseOptions(root, out)

	if _, err := Build(opts); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(opts); err == nil {
		t.Fatal("已存在的输出路径应被拒绝")
	}
	opts.Force = true
	if _, err := Build(opts); err != nil {
		t.Fatalf("--force 应允许覆盖: %v", err)
	}
}

func TestZipArchiveAndRelativeEntries(t *testing.T) {
	root := fixtureTree(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "delivery")
	zipPath := filepath.Join(parent, "archive.zip")
	opts := baseOptions(root, out)
	opts.ZipPath = zipPath

	res, err := Build(opts)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(res.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if filepath.IsAbs(f.Name) || strings.HasPrefix(f.Name, "/") ||
			strings.Contains(f.Name, "..") {
			t.Fatalf("zip 内出现不可移植条目: %q", f.Name)
		}
	}
}

func TestIncludeDuplicates(t *testing.T) {
	root := fixtureTree(t)
	out := filepath.Join(t.TempDir(), "delivery")
	opts := baseOptions(root, out)
	opts.IncludeDuplicates = true

	res, err := Build(opts)
	if err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(out, "subtitles", "en", "movie.en.srt")
	if _, err := os.Stat(srt); err != nil {
		t.Fatalf("IncludeDuplicates 应保留重复文件: %v", err)
	}
	dupes := 0
	for _, f := range res.Manifest.Files {
		if strings.HasSuffix(f.Path, "movie.en.srt") {
			dupes++
		}
	}
	if dupes != 1 {
		t.Fatalf("重复文件应只记录一次，得到 %d", dupes)
	}
}

func TestRejectsEmptyAudio(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "audio", "notes.txt"), "ignore")
	opts := baseOptions(root, filepath.Join(t.TempDir(), "out"))
	if _, err := Build(opts); err == nil {
		t.Fatal("没有音频文件时必须失败")
	}
}
