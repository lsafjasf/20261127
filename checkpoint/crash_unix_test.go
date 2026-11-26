//go:build unix

package checkpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
)

const (
	crashChildEnv = "RENDER_CHECKPOINT_CRASH_CHILD"
	crashDirEnv   = "RENDER_CHECKPOINT_CRASH_DIR"
)

func crashChildMain() {
	dir := os.Getenv(crashDirEnv)
	store, err := OpenStore(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	tasks := []Task{
		{
			ID:      "chapter-1",
			Version: "crash-v1",
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				return rc.PutBytes(ctx, []byte("durable chapter one"), "text/plain")
			},
		},
		{
			ID:        "chapter-2",
			DependsOn: []string{"chapter-1"},
			Version:   "crash-v1",
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				partial := filepath.Join(store.stagingDir, "power-loss.part")
				if err := os.WriteFile(partial, []byte("half-rendered"), 0o600); err != nil {
					return nil, err
				}
				_ = ctx
				if err := syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
					return nil, err
				}
				select {}
			},
		},
	}

	runner := &Runner{Store: store, JobID: "crash-book", Tasks: tasks, Concurrency: 1}
	if _, err := runner.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
}

func TestSIGKILLPowerLossRestartsFromCheckpoints(t *testing.T) {
	if os.Getenv(crashChildEnv) == "1" {
		crashChildMain()
		return
	}

	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestSIGKILLPowerLossRestartsFromCheckpoints$", "-test.count=1")
	cmd.Env = append(os.Environ(), crashChildEnv+"=1", crashDirEnv+"="+dir)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	if err == nil {
		t.Fatalf("crash child completed normally; output=%s", output.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("child error = %v, want ExitError", err)
	}
	waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatalf("exit status type %T", exitErr.Sys())
	}
	if !waitStatus.Signaled() || waitStatus.Signal() != syscall.SIGKILL {
		t.Fatalf("child termination = %v, want SIGKILL; output=%s", waitStatus, output.String())
	}

	partials, err := filepath.Glob(filepath.Join(dir, "staging", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(partials) != 1 {
		t.Fatalf("staging after kill = %v, want one partial", partials)
	}

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Unlock() })

	var chapter1Calls, chapter2Calls atomic.Int64
	tasks := []Task{
		{
			ID:      "chapter-1",
			Version: "crash-v1",
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				chapter1Calls.Add(1)
				return rc.PutBytes(ctx, []byte("durable chapter one"), "text/plain")
			},
		},
		{
			ID:        "chapter-2",
			DependsOn: []string{"chapter-1"},
			Version:   "crash-v1",
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				chapter2Calls.Add(1)
				return rc.PutBytes(ctx, []byte("completed chapter two"), "text/plain")
			},
		},
	}

	report, err := (&Runner{Store: store, JobID: "crash-book", Tasks: tasks, Concurrency: 1}).Run(context.Background())
	if err != nil {
		t.Fatalf("restart Run: %v", err)
	}
	if chapter1Calls.Load() != 0 {
		t.Fatalf("chapter-1 was redone after SIGKILL: calls=%d report=%+v", chapter1Calls.Load(), report)
	}
	if chapter2Calls.Load() != 1 {
		t.Fatalf("chapter-2 calls = %d, want one rebuild", chapter2Calls.Load())
	}
	if len(report.Reused) != 1 || report.Reused[0] != "chapter-1" {
		t.Fatalf("reused = %v, want chapter-1", report.Reused)
	}
	if report.Recovery == nil || len(report.Recovery.QuarantinePaths) == 0 {
		t.Fatalf("recovery report = %+v, want quarantined partial", report.Recovery)
	}
	content, err := os.ReadFile(report.Recovery.QuarantinePaths[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "half-rendered" {
		t.Fatalf("quarantined bytes = %q", content)
	}
}
