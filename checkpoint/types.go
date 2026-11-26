package checkpoint

import (
	"errors"
	"time"
)

// Status is the persisted state of one checkpointed graph node.
type Status string

const (
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusCanceled    Status = "canceled"
	StatusInterrupted Status = "interrupted"
)

var (
	ErrInvalidJobID       = errors.New("invalid checkpoint job id")
	ErrInvalidArtifactID  = errors.New("invalid artifact digest")
	ErrArtifactNotFound   = errors.New("artifact not found")
	ErrArtifactCorrupt    = errors.New("artifact checksum mismatch")
	ErrCheckpointLocked   = errors.New("checkpoint store is locked by another process")
	ErrDAGDuplicateNode   = errors.New("duplicate task id")
	ErrDAGMissingDep      = errors.New("task depends on unknown node")
	ErrDAGCycle           = errors.New("dependency cycle detected")
	ErrDependencyMissing  = errors.New("dependency artifact is unavailable")
	ErrTaskReturnedNoFile = errors.New("task completed without returning an artifact")
)

// Artifact is the immutable, checksummed result of a task.
type Artifact struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type,omitempty"`
}

// DependencyRef records the exact artifact consumed by a completed node.
type DependencyRef struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// NodeRecord is one segment checkpoint.
type NodeRecord struct {
	ID           string                   `json:"id"`
	Status       Status                   `json:"status"`
	CacheKey     string                   `json:"cache_key"`
	Artifact     *Artifact                `json:"artifact,omitempty"`
	Dependencies map[string]DependencyRef `json:"dependencies,omitempty"`
	Attempt      int                      `json:"attempt"`
	LastError    string                   `json:"last_error,omitempty"`
	StartedAt    *time.Time               `json:"started_at,omitempty"`
	FinishedAt   *time.Time               `json:"finished_at,omitempty"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

// Checkpoint is the durable state for one rendering job.
type Checkpoint struct {
	Version   int                    `json:"version"`
	JobID     string                 `json:"job_id"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Nodes     map[string]*NodeRecord `json:"nodes"`
}

// RecoveryReport describes outputs moved away during startup recovery.
type RecoveryReport struct {
	StagingFiles     []string `json:"staging_files,omitempty"`
	CorruptArtifacts []string `json:"corrupt_artifacts,omitempty"`
	MissingArtifacts []string `json:"missing_artifacts,omitempty"`
	InterruptedNodes []string `json:"interrupted_nodes,omitempty"`
	QuarantinePaths  []string `json:"quarantine_paths,omitempty"`
}
