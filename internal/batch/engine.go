package batch

import (
	"errors"
	"path/filepath"
)

// Hook points at which a simulated crash or failure can be injected.
const (
	HookAfterFragmentStage = "after_fragment_stage"
	HookAfterFragmentWAL   = "after_fragment_wal"
	HookAfterAssemble      = "after_assemble"
	HookAfterFileWAL       = "after_file_wal"
	HookAfterManifestWrite = "after_manifest_write"
	HookAfterBatchWAL      = "after_batch_wal"
)

// HookFunc is invoked after each named stage. Returning an error aborts the
// run as if a real failure happened at that point. Tests use it to prove that
// every intermediate state is recoverable.
type HookFunc func(hook string, file FileSpec, fragment int) error

// Config configures an Engine.
type Config struct {
	// OutputDir holds the WAL, staging area, files and manifest.
	OutputDir string
	// Renderer is the deterministic fragment renderer.
	Renderer Renderer
	// Hook is optional and must be nil outside of failure-injection tests.
	Hook HookFunc
}

// Engine runs crash-safe, resumable batch renders.
type Engine struct {
	cfg Config
}

// NewEngine validates and constructs an Engine.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.OutputDir == "" {
		return nil, errors.New("OutputDir is required")
	}
	if cfg.Renderer == nil {
		return nil, errors.New("Renderer is required")
	}
	return &Engine{cfg: cfg}, nil
}

// RunResult summarizes one invocation.
type RunResult struct {
	BatchID  string
	Manifest Manifest
	// FragmentsRendered counts fragments actually rendered in this run;
	// fragments satisfied from the WAL are not counted.
	FragmentsRendered int
	Resumed           bool
}

func (e *Engine) walDir() string   { return filepath.Join(e.cfg.OutputDir, "txn") }
func (e *Engine) stageDir() string { return filepath.Join(e.cfg.OutputDir, "stage") }
func (e *Engine) filesDir() string { return filepath.Join(e.cfg.OutputDir, "files") }

func (e *Engine) hook(name string, file FileSpec, fragment int) error {
	if e.cfg.Hook == nil {
		return nil
	}
	return e.cfg.Hook(name, file, fragment)
}
