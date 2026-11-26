package batch

import (
	"errors"
	"fmt"
	"os"
)

// Run executes (or resumes) one batch. It is safe to call repeatedly:
// committed fragments are skipped via the WAL, only unfinished work is
// redone, and deterministic rendering keeps every hash identical across runs.
func (e *Engine) Run(spec Spec) (RunResult, error) {
	lock, err := AcquireLock(e.cfg.OutputDir)
	if err != nil {
		return RunResult{}, err
	}
	defer lock.Release()

	for _, dir := range []string{e.walDir(), e.stageDir(), e.filesDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return RunResult{}, err
		}
	}

	specHash, err := CanonicalSpecHash(spec)
	if err != nil {
		return RunResult{}, err
	}

	// Phase 0: recover. Replay the WAL into a projection and trim any torn
	// tail left by an interrupted append.
	state, validBytes, err := replayWAL(e.walDir())
	if err != nil {
		if !errors.Is(err, ErrCorruptWAL) {
			return RunResult{}, err
		}
		if truncErr := truncateWALAt(e.walDir(), validBytes); truncErr != nil {
			return RunResult{}, truncErr
		}
	}

	log, err := openWAL(e.walDir())
	if err != nil {
		return RunResult{}, err
	}
	defer log.Close()

	result := RunResult{Resumed: state.batchID != ""}
	if state.batchID == "" {
		batchID := specHash[:16]
		if err := log.append(walRecord{Type: recBatchOpen, BatchID: batchID, SpecHash: specHash}); err != nil {
			return RunResult{}, err
		}
		state.batchID = batchID
		state.specHash = specHash
	} else if state.specHash != specHash {
		return RunResult{}, fmt.Errorf(
			"spec hash mismatch: output directory already serves spec %s, got %s",
			state.specHash, specHash)
	}
	result.BatchID = state.batchID

	if state.committed {
		m, manifestErr := ReadManifest(e.cfg.OutputDir)
		if manifestErr != nil {
			return RunResult{}, manifestErr
		}
		result.Manifest = m
		return result, nil
	}

	entries, rendered, err := e.processFiles(spec, state, log)
	if err != nil {
		return RunResult{}, err
	}

	// Phase 4: commit the batch. The manifest is replaced atomically first;
	// only then does the batch_commit record make the run final.
	manifest := BuildManifest(state.batchID, specHash, entries)
	if err := writeManifest(e.cfg.OutputDir, manifest); err != nil {
		return RunResult{}, err
	}
	if err := e.hook(HookAfterManifestWrite, FileSpec{}, -1); err != nil {
		return RunResult{}, err
	}
	if err := log.append(walRecord{Type: recBatchCommit, Manifest: manifest.Digest}); err != nil {
		return RunResult{}, err
	}
	if err := e.hook(HookAfterBatchWAL, FileSpec{}, -1); err != nil {
		return RunResult{}, err
	}

	result.Manifest = manifest
	result.FragmentsRendered = rendered
	return result, nil
}
