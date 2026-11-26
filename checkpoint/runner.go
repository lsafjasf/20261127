package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
)

type EventType string

const (
	EventRecovery  EventType = "recovery"
	EventReused    EventType = "reused"
	EventStarted   EventType = "started"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
	EventCanceled  EventType = "canceled"
	EventSkipped   EventType = "skipped"
)

// Event is an optional progress notification.
type Event struct {
	Type     EventType
	JobID    string
	TaskID   string
	Attempt  int
	Artifact *Artifact
	Recovery *RecoveryReport
	Err      error
	Elapsed  time.Duration
}

// Runner executes a checkpointed dependency graph.
type Runner struct {
	Store       *Store
	JobID       string
	Tasks       []Task
	Concurrency int
	OnEvent     func(Event)
	eventMu     sync.Mutex
}

// Report summarizes one Run invocation.
type Report struct {
	JobID    string
	Recovery *RecoveryReport
	Ran      []string
	Reused   []string
	Canceled []string
	Skipped  []string
	Failed   []string
}

// Run recovers state, verifies reusable nodes, and executes missing or stale
// nodes in dependency order. Completed checkpoints remain valid after ctx is
// canceled or the process is terminated.
func (r *Runner) Run(ctx context.Context) (*Report, error) {
	if r == nil || r.Store == nil {
		return nil, errors.New("checkpoint Runner requires a Store")
	}
	if err := safeID(r.JobID); err != nil {
		return nil, err
	}
	if err := r.Store.Lock(ctx); err != nil {
		return nil, err
	}
	defer func() { _ = r.Store.Unlock() }()

	checkpoint, recovery, err := r.Store.Recover(ctx, r.JobID)
	if err != nil {
		return nil, fmt.Errorf("recover checkpoint store: %w", err)
	}
	graph, err := buildGraph(r.Tasks)
	if err != nil {
		return nil, err
	}

	concurrency := r.Concurrency
	if concurrency <= 0 {
		concurrency = min(4, max(1, runtime.GOMAXPROCS(0)))
	}
	if concurrency > len(r.Tasks) && len(r.Tasks) > 0 {
		concurrency = len(r.Tasks)
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	scheduler := &scheduler{
		runner:     r,
		ctx:        runCtx,
		cancel:     cancel,
		graph:      graph,
		checkpoint: checkpoint,
		recovery:   recovery,
		outputs:    make(map[string]*Artifact, len(r.Tasks)),
		report: &Report{
			JobID:    r.JobID,
			Recovery: recovery,
		},
		sem: make(chan struct{}, concurrency),
	}
	scheduler.emit(Event{Type: EventRecovery, JobID: r.JobID, Recovery: recovery})

	for _, id := range graph.roots {
		scheduler.schedule(id)
	}
	scheduler.wg.Wait()

	if scheduler.runErr != nil {
		return scheduler.report, scheduler.runErr
	}
	if ctx.Err() != nil {
		return scheduler.report, ctx.Err()
	}
	if cause := context.Cause(runCtx); cause != nil && !errors.Is(cause, context.Canceled) {
		return scheduler.report, cause
	}
	return scheduler.report, nil
}

func (r *Runner) emit(event Event) {
	if r.OnEvent == nil {
		return
	}
	if event.JobID == "" {
		event.JobID = r.JobID
	}
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	r.OnEvent(event)
}

func (s *scheduler) emit(event Event) {
	s.runner.emit(event)
}

type scheduler struct {
	runner     *Runner
	ctx        context.Context
	cancel     context.CancelCauseFunc
	graph      *graph
	checkpoint *Checkpoint
	recovery   *RecoveryReport
	outputs    map[string]*Artifact
	report     *Report
	sem        chan struct{}
	wg         sync.WaitGroup

	schedMu sync.Mutex
	cpMu    sync.Mutex
	runErr  error
}

func (s *scheduler) schedule(id string) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		select {
		case s.sem <- struct{}{}:
		case <-s.ctx.Done():
			s.markSkipped(id)
			s.release(id)
			return
		}
		defer func() { <-s.sem }()

		s.execute(id)
		s.release(id)
	}()
}

func (s *scheduler) release(id string) {
	s.schedMu.Lock()
	ready := make([]string, 0)
	for _, child := range s.graph.dependents[id] {
		s.graph.indegree[child]--
		if s.graph.indegree[child] == 0 {
			ready = append(ready, child)
		}
	}
	s.schedMu.Unlock()

	for _, child := range ready {
		s.schedule(child)
	}
}

func (s *scheduler) markSkipped(id string) {
	s.schedMu.Lock()
	s.report.Skipped = append(s.report.Skipped, id)
	s.schedMu.Unlock()
	s.runner.emit(Event{Type: EventSkipped, JobID: s.runner.JobID, TaskID: id})
}

func (s *scheduler) fail(err error) {
	s.schedMu.Lock()
	if s.runErr == nil {
		s.runErr = err
	}
	s.schedMu.Unlock()
	s.cancel(err)
}
