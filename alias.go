// Package rendercheckpoint is the root facade for the checkpointed renderer.
// Most implementations live in ./checkpoint and ./render; the aliases here let
// callers use the common API directly from the module root.
package rendercheckpoint

import (
	"rendercheckpoint/checkpoint"
	"rendercheckpoint/render"
)

type (
	Store          = checkpoint.Store
	Runner         = checkpoint.Runner
	Task           = checkpoint.Task
	TaskFunc       = checkpoint.TaskFunc
	RunContext     = checkpoint.RunContext
	Artifact       = checkpoint.Artifact
	Checkpoint     = checkpoint.Checkpoint
	NodeRecord     = checkpoint.NodeRecord
	Report         = checkpoint.Report
	Event          = checkpoint.Event
	EventType      = checkpoint.EventType
	Status         = checkpoint.Status
	RecoveryReport = checkpoint.RecoveryReport

	Pipeline = render.Pipeline
	Chapter  = render.Chapter
	Manifest = render.Manifest
	Stage    = render.Stage
)

var OpenStore = checkpoint.OpenStore

const (
	StatusRunning     = checkpoint.StatusRunning
	StatusCompleted   = checkpoint.StatusCompleted
	StatusFailed      = checkpoint.StatusFailed
	StatusCanceled    = checkpoint.StatusCanceled
	StatusInterrupted = checkpoint.StatusInterrupted

	StageScript = render.StageScript
	StageVisual = render.StageVisual
	StageEncode = render.StageEncode

	EventRecovery  = checkpoint.EventRecovery
	EventReused    = checkpoint.EventReused
	EventStarted   = checkpoint.EventStarted
	EventCompleted = checkpoint.EventCompleted
	EventFailed    = checkpoint.EventFailed
	EventCanceled  = checkpoint.EventCanceled
	EventSkipped   = checkpoint.EventSkipped
)

var (
	ErrInvalidJobID       = checkpoint.ErrInvalidJobID
	ErrArtifactNotFound   = checkpoint.ErrArtifactNotFound
	ErrArtifactCorrupt    = checkpoint.ErrArtifactCorrupt
	ErrDAGCycle           = checkpoint.ErrDAGCycle
	ErrDAGMissingDep      = checkpoint.ErrDAGMissingDep
	ErrDAGDuplicateNode   = checkpoint.ErrDAGDuplicateNode
	ErrDependencyMissing  = checkpoint.ErrDependencyMissing
	ErrTaskReturnedNoFile = checkpoint.ErrTaskReturnedNoFile
)
