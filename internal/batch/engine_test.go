package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testSpec() Spec {
	return Spec{
		Name: "demo",
		Files: []FileSpec{
			{Path: "report/a.bin", Fragments: 3, FragmentSize: 64, Size: 200},
			{Path: "report/b.bin", Fragments: 2, FragmentSize: 128, Size: 200},
			{Path: "single.bin", Fragments: 1, FragmentSize: 96, Size: 96},
		},
	}
}

func newTestEngine(t *testing.T, hook HookFunc) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	engine, err := NewEngine(Config{
		OutputDir: dir,
		Renderer:  NewDeterministicRenderer(),
		Hook:      hook,
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine, dir
}

func TestHappyPathCommitsAllFiles(t *testing.T) {
	engine, dir := newTestEngine(t, nil)
	result, err := engine.Run(testSpec())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FragmentsRendered != 6 {
		t.Fatalf("rendered %d fragments, want 6", result.FragmentsRendered)
	}
	if result.Resumed {
		t.Fatal("first run must not be marked resumed")
	}

	manifest, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if !VerifyDigest(manifest) {
		t.Fatal("manifest digest failed verification")
	}
	if len(manifest.Files) != 3 {
		t.Fatalf("manifest has %d files, want 3", len(manifest.Files))
	}

	for _, entry := range manifest.Files {
		raw, err := os.ReadFile(filepath.Join(dir, "files", entry.Path))
		if err != nil {
			t.Fatalf("read output %s: %v", entry.Path, err)
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != entry.SHA256 {
			t.Fatalf("output hash mismatch for %s", entry.Path)
		}
	}
}

func TestCrashThenResumeOnlyRendersMissingFragments(t *testing.T) {
	spec := testSpec()

	// First attempt crashes after the second fragment's WAL record lands.
	var crashCount int
	crashHook := func(hook string, _ FileSpec, _ int) error {
		if hook == HookAfterFragmentWAL {
			crashCount++
			if crashCount == 2 {
				return errors.New("simulated crash")
			}
		}
		return nil
	}
	first, dir := newTestEngine(t, crashHook)
	if _, err := first.Run(spec); err == nil {
		t.Fatal("expected the first run to fail")
	}

	// Staging area must contain exactly the two committed fragments.
	stage, err := os.ReadDir(filepath.Join(dir, "stage"))
	if err != nil {
		t.Fatalf("read stage: %v", err)
	}
	if len(stage) != 2 {
		t.Fatalf("stage holds %d entries, want 2 durable fragments", len(stage))
	}

	// Resume with no failure injection.
	resume, err := NewEngine(Config{
		OutputDir: dir,
		Renderer:  NewDeterministicRenderer(),
	})
	if err != nil {
		t.Fatalf("resume engine: %v", err)
	}
	result, err := resume.Run(spec)
	if err != nil {
		t.Fatalf("resume Run: %v", err)
	}
	if !result.Resumed {
		t.Fatal("resumed run must be flagged")
	}
	if result.FragmentsRendered != 4 {
		t.Fatalf("resume rendered %d fragments, want only the 4 missing", result.FragmentsRendered)
	}

	// A third run must be a pure no-op.
	again, err := resume.Run(spec)
	if err != nil {
		t.Fatalf("idempotent run: %v", err)
	}
	if again.FragmentsRendered != 0 {
		t.Fatalf("third run rendered %d fragments, want 0", again.FragmentsRendered)
	}
	if again.Manifest.Digest != result.Manifest.Digest {
		t.Fatal("manifest digest changed across reruns")
	}
}
