package batch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentRunsProduceIdenticalHashes(t *testing.T) {
	spec := testSpec()
	dirA, dirB := t.TempDir(), t.TempDir()

	engineA, err := NewEngine(Config{OutputDir: dirA, Renderer: NewDeterministicRenderer()})
	if err != nil {
		t.Fatal(err)
	}
	engineB, err := NewEngine(Config{OutputDir: dirB, Renderer: NewDeterministicRenderer()})
	if err != nil {
		t.Fatal(err)
	}
	resultA, err := engineA.Run(spec)
	if err != nil {
		t.Fatal(err)
	}
	resultB, err := engineB.Run(spec)
	if err != nil {
		t.Fatal(err)
	}

	if resultA.Manifest.Digest != resultB.Manifest.Digest {
		t.Fatalf("digests diverge: %s vs %s", resultA.Manifest.Digest, resultB.Manifest.Digest)
	}

	manifestA := filepath.Join(dirA, ManifestName)
	manifestB := filepath.Join(dirB, ManifestName)
	rawA, _ := os.ReadFile(manifestA)
	rawB, _ := os.ReadFile(manifestB)
	if string(rawA) != string(rawB) {
		t.Fatal("manifest bytes must be identical for identical specs")
	}

	// Reordering files in the spec must not change the canonical hash.
	reordered := spec
	reordered.Files = append([]FileSpec(nil), spec.Files...)
	reordered.Files[0], reordered.Files[2] = reordered.Files[2], reordered.Files[0]
	dirC := t.TempDir()
	engineC, _ := NewEngine(Config{OutputDir: dirC, Renderer: NewDeterministicRenderer()})
	resultC, err := engineC.Run(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if resultC.Manifest.Digest != resultA.Manifest.Digest {
		t.Fatal("spec file ordering must not affect the manifest digest")
	}
}

func TestFlakyRendererRetriesPreserveHashes(t *testing.T) {
	spec := testSpec()
	failures := map[string]int{
		"report/a.bin#0": 2,
		"report/b.bin#1": 1,
	}

	dir := t.TempDir()
	flaky := NewFlakyRenderer(NewDeterministicRenderer(), failures)
	engine, err := NewEngine(Config{OutputDir: dir, Renderer: flaky})
	if err != nil {
		t.Fatal(err)
	}

	// First attempt fails as the renderer injects errors.
	if _, err := engine.Run(spec); err == nil {
		t.Fatal("expected injected failure on first attempt")
	}

	reliable, _ := NewEngine(Config{
		OutputDir: dir,
		Renderer:  NewDeterministicRenderer(),
	})
	resumed, err := reliable.Run(spec)
	if err != nil {
		t.Fatalf("resume after flaky failures: %v", err)
	}

	reference, _ := NewEngine(Config{
		OutputDir: t.TempDir(),
		Renderer:  NewDeterministicRenderer(),
	})
	baseline, err := reference.Run(spec)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Manifest.Digest != baseline.Manifest.Digest {
		t.Fatal("retry path produced different hashes than a clean run")
	}
}

func TestLockPreventsConcurrentEngines(t *testing.T) {
	dir := t.TempDir()
	lock, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer lock.Release()

	if _, err := AcquireLock(dir); err != ErrLocked {
		t.Fatalf("second lock error = %v, want ErrLocked", err)
	}
}

func TestTornWALTailIsTruncatedAndReplayed(t *testing.T) {
	spec := testSpec()
	dir := t.TempDir()

	// Commit one fragment through a real engine, then corrupt the log tail.
	hook := func(hook string, _ FileSpec, _ int) error {
		if hook == HookAfterFragmentWAL {
			return errStop
		}
		return nil
	}
	first, _ := NewEngine(Config{OutputDir: dir, Renderer: NewDeterministicRenderer(), Hook: hook})
	if _, err := first.Run(spec); err != errStop {
		t.Fatalf("err = %v, want errStop", err)
	}

	logPath := filepath.Join(dir, "txn", "txn.log")
	// Append garbage to emulate a torn, never-fsynced tail.
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"half":"written`); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	second, _ := NewEngine(Config{OutputDir: dir, Renderer: NewDeterministicRenderer()})
	result, err := second.Run(spec)
	if err != nil {
		t.Fatalf("resume with torn tail: %v", err)
	}
	if result.FragmentsRendered != 5 {
		t.Fatalf("rendered %d fragments, want 5 (one survived in WAL)", result.FragmentsRendered)
	}

	if _, _, err := replayWAL(filepath.Join(dir, "txn")); err != nil {
		t.Fatalf("WAL must replay cleanly after torn-tail recovery: %v", err)
	}
	// The garbage bytes must be gone: the log now ends in a valid batch commit.
	inspection, _, err := InspectWAL(filepath.Join(dir, "txn"))
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Committed {
		t.Fatal("recovered batch must be committed")
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `{"half":"written`) {
		t.Fatal("torn garbage tail survived recovery")
	}
}

func TestUnsafePathsAreRejected(t *testing.T) {
	for _, path := range []string{"../escape.bin", "/abs/bin", "..", ""} {
		if err := ValidatePath(path); err == nil {
			t.Fatalf("path %q must be rejected", path)
		}
		if err := ValidatePath(strings.TrimSpace(path)); err == nil && path != "" {
			t.Fatalf("trimmed path %q must be rejected", path)
		}
	}
}

var errStop = errors.New("stop")
