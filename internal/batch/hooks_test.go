package batch

import (
	"fmt"
	"testing"
)

func TestRecoveryFromEveryHookPoint(t *testing.T) {
	hooks := []string{
		HookAfterFragmentStage,
		HookAfterFragmentWAL,
		HookAfterAssemble,
		HookAfterFileWAL,
		HookAfterManifestWrite,
	}

	baselineEngine, err := NewEngine(Config{
		OutputDir: t.TempDir(),
		Renderer:  NewDeterministicRenderer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := baselineEngine.Run(testSpec())
	if err != nil {
		t.Fatal(err)
	}

	for _, crashHook := range hooks {
		t.Run(crashHook, func(t *testing.T) {
			dir := t.TempDir()
			once := false
			crashing, _ := NewEngine(Config{
				OutputDir: dir,
				Renderer:  NewDeterministicRenderer(),
				Hook: func(name string, _ FileSpec, _ int) error {
					if !once && name == crashHook {
						once = true
						return fmt.Errorf("simulated crash at %s", name)
					}
					return nil
				},
			})
			if _, err := crashing.Run(testSpec()); err == nil {
				t.Fatal("crash hook did not fire")
			}

			resume, _ := NewEngine(Config{
				OutputDir: dir,
				Renderer:  NewDeterministicRenderer(),
			})
			result, err := resume.Run(testSpec())
			if err != nil {
				t.Fatalf("recovery failed: %v", err)
			}
			if result.Manifest.Digest != baseline.Manifest.Digest {
				t.Fatalf("digest after recovery at %s diverges", crashHook)
			}

			// One more run must be a no-op and keep the digest stable.
			again, err := resume.Run(testSpec())
			if err != nil || again.FragmentsRendered != 0 ||
				again.Manifest.Digest != baseline.Manifest.Digest {
				t.Fatalf("post-recovery run not idempotent: %+v err=%v", again, err)
			}
		})
	}
}
