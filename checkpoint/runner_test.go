package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Unlock() })
	return store
}

func byteTask(id string, data string, counts map[string]*atomic.Int64, deps ...string) Task {
	if counts[id] == nil {
		counts[id] = &atomic.Int64{}
	}
	return Task{
		ID:        id,
		DependsOn: deps,
		Version:   "test-v1",
		Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
			counts[id].Add(1)
			content := data
			if len(deps) > 0 {
				dep, err := rc.Dependency(deps[0])
				if err != nil {
					return nil, err
				}
				content = content + ":" + dep.Digest
			}
			return rc.PutBytes(ctx, []byte(content), "text/plain")
		},
	}
}

func waitSignal[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func containsID(ids []string, id string) bool {
	return slices.Contains(ids, id)
}

func TestCancelKeepsCompletedChapterCheckpoint(t *testing.T) {
	store := newTestStore(t)
	var firstCount, blockedCount, lastCount atomic.Int64
	started := make(chan struct{}, 1)

	tasks := []Task{
		{
			ID:      "chapter-1",
			Version: "render-v1",
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				firstCount.Add(1)
				return rc.PutBytes(ctx, []byte("chapter-1-complete"), "text/plain")
			},
		},
		{
			ID:        "chapter-2",
			DependsOn: []string{"chapter-1"},
			Version:   "render-v1",
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				attempt := blockedCount.Add(1)
				if attempt == 1 {
					started <- struct{}{}
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return rc.PutBytes(ctx, []byte("chapter-2-resumed"), "text/plain")
			},
		},
		{
			ID:        "chapter-3",
			DependsOn: []string{"chapter-2"},
			Version:   "render-v1",
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				lastCount.Add(1)
				return rc.PutBytes(ctx, []byte("chapter-3"), "text/plain")
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		runner := &Runner{Store: store, JobID: "book", Tasks: tasks, Concurrency: 1}
		_, err := runner.Run(ctx)
		firstDone <- err
	}()

	waitSignal(t, started, "chapter-2 start")
	cancel()
	if err := waitSignal(t, firstDone, "cancelled run"); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Run error = %v, want context.Canceled", err)
	}
	if got := firstCount.Load(); got != 1 {
		t.Fatalf("chapter-1 executions after cancel = %d, want 1", got)
	}
	if got := lastCount.Load(); got != 0 {
		t.Fatalf("chapter-3 executions after cancel = %d, want 0", got)
	}

	resumeCtx, resumeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer resumeCancel()
	resume := &Runner{Store: store, JobID: "book", Tasks: tasks, Concurrency: 1}
	report, err := resume.Run(resumeCtx)
	if err != nil {
		t.Fatalf("resume Run: %v", err)
	}
	if !containsID(report.Reused, "chapter-1") {
		t.Fatalf("chapter-1 was not reused: reused=%v ran=%v", report.Reused, report.Ran)
	}
	if !containsID(report.Ran, "chapter-2") || !containsID(report.Ran, "chapter-3") {
		t.Fatalf("resume did not rerun pending chapters: ran=%v reused=%v", report.Ran, report.Reused)
	}
	if got := firstCount.Load(); got != 1 {
		t.Fatalf("chapter-1 redone after resume: count=%d", got)
	}
	if got := blockedCount.Load(); got != 2 {
		t.Fatalf("chapter-2 executions = %d, want canceled attempt plus resume", got)
	}
	if got := lastCount.Load(); got != 1 {
		t.Fatalf("chapter-3 executions = %d, want 1", got)
	}
}

func TestPowerLossQuarantinesPartialAndRecoversDependencies(t *testing.T) {
	store := newTestStore(t)
	counts := map[string]*atomic.Int64{}
	firstTasks := []Task{
		byteTask("ch1", "one", counts),
		byteTask("ch2", "two", counts, "ch1"),
	}
	first := &Runner{Store: store, JobID: "power-book", Tasks: firstTasks, Concurrency: 1}
	if _, err := first.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	partialPath := filepath.Join(store.stagingDir, "artifact-crash.part")
	if err := os.WriteFile(partialPath, []byte("partial-render-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := store.LoadCheckpoint("power-book")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	checkpoint.Nodes["ch3"] = &NodeRecord{ID: "ch3", Status: StatusRunning, Attempt: 1, StartedAt: &now}
	if err := store.SaveCheckpoint(checkpoint); err != nil {
		t.Fatal(err)
	}

	allTasks := []Task{
		byteTask("ch1", "one", counts),
		byteTask("ch2", "two", counts, "ch1"),
		byteTask("ch3", "three", counts, "ch2"),
		byteTask("ch4", "four", counts, "ch3"),
	}
	restarted := &Runner{Store: store, JobID: "power-book", Tasks: allTasks, Concurrency: 1}
	report, err := restarted.Run(context.Background())
	if err != nil {
		t.Fatalf("restarted Run: %v", err)
	}

	if report.Recovery == nil || len(report.Recovery.QuarantinePaths) != 1 {
		t.Fatalf("recovery report = %+v, want one quarantined file", report.Recovery)
	}
	quarantined, err := os.ReadFile(report.Recovery.QuarantinePaths[0])
	if err != nil {
		t.Fatalf("read quarantine: %v", err)
	}
	if string(quarantined) != "partial-render-bytes" {
		t.Fatalf("quarantined bytes = %q", quarantined)
	}
	if entries, _ := os.ReadDir(store.stagingDir); len(entries) != 0 {
		t.Fatalf("staging not empty after recovery: %d entries", len(entries))
	}
	if !containsID(report.Recovery.InterruptedNodes, "ch3") {
		t.Fatalf("interrupted nodes = %v, want ch3", report.Recovery.InterruptedNodes)
	}
	for _, id := range []string{"ch1", "ch2"} {
		if !containsID(report.Reused, id) {
			t.Fatalf("%s not reused: %+v", id, report)
		}
		if got := counts[id].Load(); got != 1 {
			t.Fatalf("%s count = %d, want 1 after power-loss restart", id, got)
		}
	}
	for _, id := range []string{"ch3", "ch4"} {
		if !containsID(report.Ran, id) {
			t.Fatalf("%s not run after recovery: %+v", id, report)
		}
	}
}

func TestDependencyChangeRerunsDownstreamOnly(t *testing.T) {
	store := newTestStore(t)
	var sourceCount, renderCount, finalCount, otherCount atomic.Int64

	makeTasks := func(color string) []Task {
		return []Task{
			{
				ID:      "source",
				Version: "source-v1",
				Inputs:  struct{ Color string }{Color: color},
				Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
					sourceCount.Add(1)
					return rc.PutBytes(ctx, []byte("source:"+color), "text/plain")
				},
			},
			{
				ID:        "render",
				DependsOn: []string{"source"},
				Version:   "render-v1",
				Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
					renderCount.Add(1)
					dep, _, err := rc.OpenDependency("source")
					if err != nil {
						return nil, err
					}
					defer dep.Close()
					return rc.PutReader(ctx, dep, "application/octet-stream")
				},
			},
			{
				ID:        "final",
				DependsOn: []string{"render"},
				Version:   "final-v1",
				Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
					finalCount.Add(1)
					art, err := rc.Dependency("render")
					if err != nil {
						return nil, err
					}
					return rc.PutBytes(ctx, []byte("final:"+art.Digest), "text/plain")
				},
			},
			{
				ID:      "other-chapter",
				Version: "other-v1",
				Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
					otherCount.Add(1)
					return rc.PutBytes(ctx, []byte("unrelated"), "text/plain")
				},
			},
		}
	}

	first := &Runner{Store: store, JobID: "deps", Tasks: makeTasks("blue"), Concurrency: 1}
	if _, err := first.Run(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	second := &Runner{Store: store, JobID: "deps", Tasks: makeTasks("red"), Concurrency: 1}
	report, err := second.Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	for _, id := range []string{"source", "render", "final"} {
		if !containsID(report.Ran, id) {
			t.Errorf("%s did not rerun after dependency change: ran=%v reused=%v", id, report.Ran, report.Reused)
		}
	}
	if !containsID(report.Reused, "other-chapter") {
		t.Errorf("unrelated chapter not reused: %+v", report)
	}
	if sourceCount.Load() != 2 || renderCount.Load() != 2 || finalCount.Load() != 2 {
		t.Errorf("downstream counts = %d,%d,%d want 2,2,2", sourceCount.Load(), renderCount.Load(), finalCount.Load())
	}
	if otherCount.Load() != 1 {
		t.Errorf("other chapter count = %d, want 1", otherCount.Load())
	}
}

func TestCorruptArtifactIsQuarantinedAndRebuilt(t *testing.T) {
	store := newTestStore(t)
	var aCount, bCount atomic.Int64
	tasks := []Task{
		{ID: "a", Version: "v1", Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
			aCount.Add(1)
			return rc.PutBytes(ctx, []byte("deterministic-a"), "text/plain")
		}},
		{ID: "b", DependsOn: []string{"a"}, Version: "v1", Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
			bCount.Add(1)
			if _, err := rc.Dependency("a"); err != nil {
				return nil, err
			}
			return rc.PutBytes(ctx, []byte("b"), "text/plain")
		}},
	}
	runner := &Runner{Store: store, JobID: "corrupt", Tasks: tasks, Concurrency: 1}
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	cp, err := store.LoadCheckpoint("corrupt")
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.objectPath(*cp.Nodes["a"].Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := (&Runner{Store: store, JobID: "corrupt", Tasks: tasks, Concurrency: 1}).Run(context.Background())
	if err != nil {
		t.Fatalf("recovery run: %v", err)
	}
	if len(report.Recovery.CorruptArtifacts) != 1 || len(report.Recovery.QuarantinePaths) != 1 {
		t.Fatalf("recovery = %+v, want corrupt artifact and quarantine", report.Recovery)
	}
	if !containsID(report.Ran, "a") {
		t.Fatalf("corrupt node not rebuilt: %+v", report)
	}
	if !containsID(report.Reused, "b") {
		t.Fatalf("identical rebuilt dependency should allow b reuse: %+v", report)
	}
	if aCount.Load() != 2 || bCount.Load() != 1 {
		t.Fatalf("counts a=%d b=%d, want 2 and 1", aCount.Load(), bCount.Load())
	}
}

func TestDAGValidation(t *testing.T) {
	_, err := buildGraph([]Task{
		{ID: "a", DependsOn: []string{"b"}, Run: func(context.Context, *RunContext) (*Artifact, error) { return nil, nil }},
		{ID: "b", DependsOn: []string{"a"}, Run: func(context.Context, *RunContext) (*Artifact, error) { return nil, nil }},
	})
	if !errors.Is(err, ErrDAGCycle) {
		t.Fatalf("cycle error = %v, want ErrDAGCycle", err)
	}

	_, err = buildGraph([]Task{{ID: "a", DependsOn: []string{"missing"}, Run: func(context.Context, *RunContext) (*Artifact, error) { return nil, nil }}})
	if !errors.Is(err, ErrDAGMissingDep) {
		t.Fatalf("missing dep error = %v, want ErrDAGMissingDep", err)
	}
}

func TestParallelGraphRunsOnceAndResumes(t *testing.T) {
	store := newTestStore(t)
	counts := make([]atomic.Int64, 12)
	tasks := make([]Task, 0, len(counts))
	for i := range counts {
		i := i
		tasks = append(tasks, Task{
			ID:        fmt.Sprintf("chapter-%02d", i+1),
			Version:   "parallel-v1",
			DependsOn: nil,
			Run: func(ctx context.Context, rc *RunContext) (*Artifact, error) {
				counts[i].Add(1)
				time.Sleep(5 * time.Millisecond)
				return rc.PutBytes(ctx, []byte(fmt.Sprintf("chapter-%d", i)), "text/plain")
			},
		})
	}
	runner := &Runner{Store: store, JobID: "parallel", Tasks: tasks, Concurrency: 4}
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("first parallel run: %v", err)
	}
	report, err := (&Runner{Store: store, JobID: "parallel", Tasks: tasks, Concurrency: 4}).Run(context.Background())
	if err != nil {
		t.Fatalf("resume parallel run: %v", err)
	}
	if len(report.Reused) != len(tasks) || len(report.Ran) != 0 {
		t.Fatalf("resume = ran %v reused %v, want all reused", report.Ran, report.Reused)
	}
	for i := range counts {
		if got := counts[i].Load(); got != 1 {
			t.Fatalf("task %d count = %d, want 1", i, got)
		}
	}
}
