package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type quarantineNote struct {
	OriginalPath  string    `json:"original_path"`
	Reason        string    `json:"reason"`
	QuarantinedAt time.Time `json:"quarantined_at"`
}

// Quarantine moves an incomplete or suspect file out of all reusable paths.
// The original bytes are retained for inspection or manual recovery.
func (s *Store) Quarantine(path, reason string) (string, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("stat quarantined file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("refusing to quarantine directory: %s", path)
	}

	base := filepath.Base(path)
	destination := filepath.Join(
		s.quarantineDir,
		fmt.Sprintf("%d-%s", time.Now().UnixNano(), base),
	)
	for suffix := 1; ; suffix++ {
		_, err := os.Stat(destination)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		destination = filepath.Join(
			s.quarantineDir,
			fmt.Sprintf("%d-%d-%s", time.Now().UnixNano(), suffix, base),
		)
	}

	if err := os.Rename(path, destination); err != nil {
		if !errors.Is(err, os.ErrInvalid) && !isCrossDevice(err) {
			return "", fmt.Errorf("move to quarantine: %w", err)
		}
		if err := copyAndRemove(path, destination); err != nil {
			return "", err
		}
	}
	if err := fsyncDir(s.quarantineDir); err != nil {
		return "", fmt.Errorf("sync quarantine directory: %w", err)
	}

	note := quarantineNote{
		OriginalPath:  path,
		Reason:        reason,
		QuarantinedAt: nowUTC(),
	}
	if data, err := json.MarshalIndent(note, "", "  "); err == nil {
		notePath := destination + ".reason.json"
		if err := os.WriteFile(notePath, data, 0o600); err == nil {
			_ = fsyncDir(s.quarantineDir)
		}
	}
	return destination, nil
}

func copyAndRemove(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open quarantine source: %w", err)
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = in.Close()
		return fmt.Errorf("create quarantine destination: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = in.Close()
		_ = out.Close()
		return fmt.Errorf("copy quarantine file: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = in.Close()
		_ = out.Close()
		return fmt.Errorf("sync quarantine file: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = in.Close()
		return fmt.Errorf("close quarantine file: %w", err)
	}
	if err := in.Close(); err != nil {
		return fmt.Errorf("close quarantine source: %w", err)
	}
	return os.Remove(source)
}

func isCrossDevice(err error) bool {
	return strings.Contains(err.Error(), "invalid cross-device link")
}

// OpenArtifact opens a committed artifact after validating its digest.
func (s *Store) OpenArtifact(artifact *Artifact) (io.ReadCloser, error) {
	if artifact == nil {
		return nil, ErrArtifactNotFound
	}
	if err := s.VerifyArtifact(artifact); err != nil {
		return nil, err
	}
	path, err := s.objectPath(*artifact)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// VerifyArtifact checks both size and SHA-256.
func (s *Store) VerifyArtifact(artifact *Artifact) error {
	if artifact == nil {
		return ErrArtifactNotFound
	}
	path, err := s.objectPath(*artifact)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrArtifactNotFound, artifact.Digest)
	}
	if err != nil {
		return fmt.Errorf("stat artifact: %w", err)
	}
	if artifact.Size >= 0 && info.Size() != artifact.Size {
		return fmt.Errorf("%w: size %d != checkpoint %d", ErrArtifactCorrupt, info.Size(), artifact.Size)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open artifact for verification: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("verify artifact: %w", err)
	}
	actual := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if actual != artifact.Digest {
		return fmt.Errorf("%w: digest %s != checkpoint %s", ErrArtifactCorrupt, actual, artifact.Digest)
	}
	return nil
}

// ExportArtifact verifies and atomically publishes an artifact under output.
func (s *Store) ExportArtifact(ctx context.Context, artifact *Artifact, relativePath string) (string, error) {
	if artifact == nil {
		return "", ErrArtifactNotFound
	}
	if err := s.VerifyArtifact(artifact); err != nil {
		return "", err
	}
	source, err := s.objectPath(*artifact)
	if err != nil {
		return "", err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	return s.ExportReader(ctx, relativePath, input)
}

// ExportReader writes a final output through staging and atomically renames it.
func (s *Store) ExportReader(ctx context.Context, relativePath string, reader io.Reader) (published string, retErr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	clean, err := safeOutputPath(relativePath)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(s.outputDir, clean)
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	if err := fsyncDirectoryChain(s.outputDir, filepath.Dir(destination)); err != nil {
		return "", fmt.Errorf("sync output directory chain: %w", err)
	}

	temp, err := os.CreateTemp(s.stagingDir, "output-*.part")
	if err != nil {
		return "", fmt.Errorf("create output staging file: %w", err)
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_, _ = s.Quarantine(tempName, "output export did not commit")
		}
	}()

	if _, err := io.Copy(temp, reader); err != nil {
		return "", fmt.Errorf("write output: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("sync output: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempName, destination); err != nil {
		return "", fmt.Errorf("publish output: %w", err)
	}
	committed = true
	if err := fsyncDir(filepath.Dir(destination)); err != nil {
		return "", fmt.Errorf("sync output directory: %w", err)
	}
	return destination, nil
}

func safeOutputPath(relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("output path must be relative: %q", relativePath)
	}
	clean := filepath.Clean(relativePath)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("output path escapes output directory: %q", relativePath)
	}
	return clean, nil
}

// Recover moves orphaned staging data away and marks interrupted checkpoint
// nodes as non-completed. Completed artifacts are checksum verified.
func (s *Store) Recover(ctx context.Context, jobID string) (*Checkpoint, *RecoveryReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := safeID(jobID); err != nil {
		return nil, nil, err
	}

	report := &RecoveryReport{}
	entries, err := os.ReadDir(s.stagingDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read staging directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(s.stagingDir, entry.Name())
		quarantined, err := s.Quarantine(path, "orphaned staging file found during startup recovery")
		if err != nil {
			return nil, nil, err
		}
		if quarantined != "" {
			report.StagingFiles = append(report.StagingFiles, path)
			report.QuarantinePaths = append(report.QuarantinePaths, quarantined)
		}
	}

	checkpoint, err := s.LoadCheckpoint(jobID)
	if err != nil {
		return nil, report, err
	}
	changed := false
	for id, node := range checkpoint.Nodes {
		if node == nil {
			continue
		}
		if node.Status == StatusRunning {
			node.Status = StatusInterrupted
			node.LastError = "process exited before node checkpoint committed"
			report.InterruptedNodes = append(report.InterruptedNodes, id)
			changed = true
		}
		if node.Status != StatusCompleted || node.Artifact == nil {
			continue
		}
		if err := s.VerifyArtifact(node.Artifact); err != nil {
			node.Status = StatusInterrupted
			node.LastError = err.Error()
			switch {
			case errors.Is(err, ErrArtifactNotFound):
				report.MissingArtifacts = append(report.MissingArtifacts, node.Artifact.Digest)
			case errors.Is(err, ErrArtifactCorrupt):
				report.CorruptArtifacts = append(report.CorruptArtifacts, node.Artifact.Digest)
				path, pathErr := s.objectPath(*node.Artifact)
				if pathErr == nil {
					quarantined, qErr := s.Quarantine(path, "committed artifact failed checksum verification")
					if qErr != nil {
						return nil, report, qErr
					}
					if quarantined != "" {
						report.QuarantinePaths = append(report.QuarantinePaths, quarantined)
					}
				}
			}
			node.Artifact = nil
			changed = true
		}
	}
	if changed {
		if err := s.SaveCheckpoint(checkpoint); err != nil {
			return nil, report, err
		}
	}
	return checkpoint, report, nil
}
