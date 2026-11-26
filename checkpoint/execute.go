package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (s *scheduler) execute(id string) {
	started := time.Now()
	task := s.graph.tasks[id]

	if err := s.ctx.Err(); err != nil {
		s.recordCanceled(id, 0, err)
		return
	}

	dependencyArtifacts, err := s.currentDependencies(task)
	if err != nil {
		s.fail(fmt.Errorf("resolve dependencies for %s: %w", id, err))
		s.recordSkipped(id)
		return
	}
	refs := dependencyRefs(task.DependsOn, dependencyArtifacts)

	cacheKey, err := task.resolvedCacheKey()
	if err != nil {
		s.fail(err)
		s.recordFailed(id, 0, err, started)
		return
	}

	s.cpMu.Lock()
	existing := s.checkpoint.Nodes[id]
	reuseErr := dependenciesMatch(existing, task.DependsOn, dependencyArtifacts)
	if reuseErr == nil {
		switch {
		case existing.CacheKey != cacheKey:
			reuseErr = fmt.Errorf("cache key changed for %s", id)
		default:
			reuseErr = s.runner.Store.VerifyArtifact(existing.Artifact)
		}
	}
	s.cpMu.Unlock()

	if reuseErr == nil {
		artifact := existing.Artifact
		s.schedMu.Lock()
		s.outputs[id] = artifact
		s.report.Reused = append(s.report.Reused, id)
		s.schedMu.Unlock()
		s.runner.emit(Event{
			Type:     EventReused,
			JobID:    s.runner.JobID,
			TaskID:   id,
			Attempt:  existing.Attempt,
			Artifact: artifact,
			Elapsed:  time.Since(started),
		})
		return
	}

	select {
	case <-s.ctx.Done():
		s.recordCanceled(id, s.currentAttempt(id), s.ctx.Err())
		return
	default:
	}

	attempt, err := s.markRunning(id, cacheKey, refs)
	if err != nil {
		s.fail(err)
		return
	}
	s.runner.emit(Event{Type: EventStarted, JobID: s.runner.JobID, TaskID: id, Attempt: attempt})

	runContext := &RunContext{
		store:        s.runner.Store,
		taskID:       id,
		dependencies: dependencyArtifacts,
	}
	artifact, taskErr := task.Run(s.ctx, runContext)
	finished := nowUTC()

	if taskErr != nil {
		if errors.Is(taskErr, context.Canceled) || errors.Is(s.ctx.Err(), context.Canceled) {
			_ = s.markTerminal(id, StatusCanceled, attempt, refs, nil, taskErr, finished)
			s.recordCanceled(id, attempt, taskErr)
			s.fail(taskErr)
			return
		}
		_ = s.markTerminal(id, StatusFailed, attempt, refs, nil, taskErr, finished)
		s.schedMu.Lock()
		s.report.Failed = append(s.report.Failed, id)
		s.schedMu.Unlock()
		s.runner.emit(Event{Type: EventFailed, JobID: s.runner.JobID, TaskID: id, Attempt: attempt, Err: taskErr})
		s.fail(fmt.Errorf("task %s: %w", id, taskErr))
		return
	}
	if artifact == nil {
		taskErr = fmt.Errorf("%w: %s", ErrTaskReturnedNoFile, id)
		_ = s.markTerminal(id, StatusFailed, attempt, refs, nil, taskErr, finished)
		s.schedMu.Lock()
		s.report.Failed = append(s.report.Failed, id)
		s.schedMu.Unlock()
		s.fail(taskErr)
		return
	}
	if err := s.runner.Store.VerifyArtifact(artifact); err != nil {
		taskErr = fmt.Errorf("task %s returned unverified artifact: %w", id, err)
		_ = s.markTerminal(id, StatusFailed, attempt, refs, nil, taskErr, finished)
		s.fail(taskErr)
		return
	}

	if err := s.markCompleted(id, cacheKey, attempt, refs, artifact, finished); err != nil {
		s.fail(err)
		return
	}

	s.schedMu.Lock()
	s.outputs[id] = artifact
	s.report.Ran = append(s.report.Ran, id)
	s.schedMu.Unlock()
	s.runner.emit(Event{
		Type:     EventCompleted,
		JobID:    s.runner.JobID,
		TaskID:   id,
		Attempt:  attempt,
		Artifact: artifact,
		Elapsed:  time.Since(started),
	})
}

func (s *scheduler) currentDependencies(task Task) (map[string]*Artifact, error) {
	artifacts := make(map[string]*Artifact, len(task.DependsOn))
	s.schedMu.Lock()
	defer s.schedMu.Unlock()
	for _, id := range task.DependsOn {
		artifact := s.outputs[id]
		if artifact == nil {
			if s.ctx.Err() != nil {
				return nil, s.ctx.Err()
			}
			if s.runErr != nil {
				return nil, s.runErr
			}
			return nil, fmt.Errorf("%w: %s", ErrDependencyMissing, id)
		}
		artifacts[id] = artifact
	}
	return artifacts, nil
}

func dependencyRefs(ids []string, artifacts map[string]*Artifact) map[string]DependencyRef {
	refs := make(map[string]DependencyRef, len(ids))
	for _, id := range ids {
		artifact := artifacts[id]
		if artifact == nil {
			continue
		}
		refs[id] = DependencyRef{Digest: artifact.Digest, Size: artifact.Size}
	}
	return refs
}

func (s *scheduler) currentAttempt(id string) int {
	s.cpMu.Lock()
	defer s.cpMu.Unlock()
	if node := s.checkpoint.Nodes[id]; node != nil {
		return node.Attempt
	}
	return 0
}

func (s *scheduler) markRunning(id, cacheKey string, refs map[string]DependencyRef) (int, error) {
	s.cpMu.Lock()
	defer s.cpMu.Unlock()

	node := s.checkpoint.Nodes[id]
	if node == nil {
		node = &NodeRecord{ID: id}
		s.checkpoint.Nodes[id] = node
	}
	now := nowUTC()
	node.Status = StatusRunning
	node.CacheKey = cacheKey
	node.Attempt++
	node.Dependencies = refs
	node.Artifact = nil
	node.LastError = ""
	node.StartedAt = &now
	node.FinishedAt = nil
	node.UpdatedAt = now
	if err := s.runner.Store.SaveCheckpoint(s.checkpoint); err != nil {
		return node.Attempt, fmt.Errorf("save running checkpoint for %s: %w", id, err)
	}
	return node.Attempt, nil
}

func (s *scheduler) markCompleted(id, cacheKey string, attempt int, refs map[string]DependencyRef, artifact *Artifact, finished time.Time) error {
	s.cpMu.Lock()
	defer s.cpMu.Unlock()

	node := s.checkpoint.Nodes[id]
	if node == nil {
		node = &NodeRecord{ID: id}
		s.checkpoint.Nodes[id] = node
	}
	node.Status = StatusCompleted
	node.CacheKey = cacheKey
	node.Attempt = attempt
	node.Dependencies = refs
	node.Artifact = artifact
	node.LastError = ""
	node.FinishedAt = &finished
	node.UpdatedAt = finished
	if err := s.runner.Store.SaveCheckpoint(s.checkpoint); err != nil {
		return fmt.Errorf("save completed checkpoint for %s: %w", id, err)
	}
	return nil
}

func (s *scheduler) markTerminal(id string, status Status, attempt int, refs map[string]DependencyRef, artifact *Artifact, taskErr error, finished time.Time) error {
	s.cpMu.Lock()
	defer s.cpMu.Unlock()

	node := s.checkpoint.Nodes[id]
	if node == nil {
		node = &NodeRecord{ID: id}
		s.checkpoint.Nodes[id] = node
	}
	node.Status = status
	node.Attempt = attempt
	if refs != nil {
		node.Dependencies = refs
	}
	if artifact != nil {
		node.Artifact = artifact
	}
	if taskErr != nil {
		node.LastError = taskErr.Error()
	}
	node.FinishedAt = &finished
	node.UpdatedAt = finished
	return s.runner.Store.SaveCheckpoint(s.checkpoint)
}

func (s *scheduler) recordCanceled(id string, attempt int, err error) {
	s.schedMu.Lock()
	s.report.Canceled = append(s.report.Canceled, id)
	s.schedMu.Unlock()
	s.runner.emit(Event{Type: EventCanceled, JobID: s.runner.JobID, TaskID: id, Attempt: attempt, Err: err})
}

func (s *scheduler) recordSkipped(id string) {
	s.schedMu.Lock()
	s.report.Skipped = append(s.report.Skipped, id)
	s.schedMu.Unlock()
	s.runner.emit(Event{Type: EventSkipped, JobID: s.runner.JobID, TaskID: id})
}

func (s *scheduler) recordFailed(id string, attempt int, err error, started time.Time) {
	s.schedMu.Lock()
	s.report.Failed = append(s.report.Failed, id)
	s.schedMu.Unlock()
	s.runner.emit(Event{Type: EventFailed, JobID: s.runner.JobID, TaskID: id, Attempt: attempt, Err: err, Elapsed: time.Since(started)})
}
