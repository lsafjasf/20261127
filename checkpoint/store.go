package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const checkpointVersion = 1

var safeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Store owns the checkpoint database, content-addressed artifacts, staging,
// output, and quarantine directories for one rendering workspace.
type Store struct {
	root           string
	checkpointsDir string
	stagingDir     string
	objectsDir     string
	outputDir      string
	quarantineDir  string
	lockPath       string
	stateMu        sync.Mutex
	lockMu         sync.Mutex
	processLock    *workspaceLock
}

// OpenStore creates the durable workspace layout when necessary.
func OpenStore(root string) (*Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve store root: %w", err)
	}

	store := &Store{
		root:           absRoot,
		checkpointsDir: filepath.Join(absRoot, "checkpoints"),
		stagingDir:     filepath.Join(absRoot, "staging"),
		objectsDir:     filepath.Join(absRoot, "objects"),
		outputDir:      filepath.Join(absRoot, "output"),
		quarantineDir:  filepath.Join(absRoot, "quarantine"),
		lockPath:       filepath.Join(absRoot, "render.lock"),
	}
	for _, dir := range []string{
		store.checkpointsDir,
		store.stagingDir,
		store.objectsDir,
		store.outputDir,
		store.quarantineDir,
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	if err := fsyncDir(absRoot); err != nil {
		return nil, fmt.Errorf("sync store root: %w", err)
	}
	return store, nil
}

// Root returns the workspace root.
func (s *Store) Root() string { return s.root }

// OutputDir returns the directory used for published final files.
func (s *Store) OutputDir() string { return s.outputDir }

func safeID(id string) error {
	if !safeIDPattern.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidJobID, id)
	}
	return nil
}

func (s *Store) checkpointPath(jobID string) (string, error) {
	if err := safeID(jobID); err != nil {
		return "", err
	}
	return filepath.Join(s.checkpointsDir, jobID+".json"), nil
}

func parseDigest(digest string) (string, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) {
		return "", fmt.Errorf("%w: missing sha256 prefix", ErrInvalidArtifactID)
	}
	sum := strings.TrimPrefix(digest, prefix)
	decoded, err := hex.DecodeString(sum)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("%w: %q", ErrInvalidArtifactID, digest)
	}
	return sum, nil
}

func (s *Store) objectPath(artifact Artifact) (string, error) {
	sum, err := parseDigest(artifact.Digest)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.objectsDir, sum), nil
}

func fsyncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	return errors.Join(syncErr, closeErr)
}

func fsyncDirectoryChain(base, target string) error {
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("directory %s is not under %s", target, base)
	}
	if err := fsyncDir(base); err != nil {
		return err
	}
	current := base
	if relative == "." {
		return nil
	}
	for _, part := range strings.Split(relative, string(os.PathSeparator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		if err := fsyncDir(current); err != nil {
			return err
		}
	}
	return nil
}

func closeFile(file *os.File) error {
	if file == nil {
		return nil
	}
	return file.Close()
}

func nowUTC() time.Time { return time.Now().UTC() }

func (s *Store) writeJSONAtomic(destination string, value any, mode fs.FileMode) (retErr error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json: %w", err)
	}

	temp, err := os.CreateTemp(s.stagingDir, ".checkpoint-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary checkpoint: %w", err)
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temp.Close()
			_, _ = s.Quarantine(tempName, "checkpoint write did not commit")
		}
	}()

	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod checkpoint: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close checkpoint: %w", err)
	}
	if err := os.Rename(tempName, destination); err != nil {
		return fmt.Errorf("commit checkpoint: %w", err)
	}
	committed = true
	if err := fsyncDir(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("sync checkpoint directory: %w", err)
	}
	return nil
}

// SaveCheckpoint atomically persists the complete job checkpoint.
func (s *Store) SaveCheckpoint(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return errors.New("checkpoint is nil")
	}
	path, err := s.checkpointPath(checkpoint.JobID)
	if err != nil {
		return err
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	checkpoint.Version = checkpointVersion
	checkpoint.UpdatedAt = nowUTC()
	if checkpoint.Nodes == nil {
		checkpoint.Nodes = make(map[string]*NodeRecord)
	}
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = checkpoint.UpdatedAt
	}
	return s.writeJSONAtomic(path, checkpoint, 0o600)
}

// LoadCheckpoint reads a checkpoint or returns a new in-memory one.
func (s *Store) LoadCheckpoint(jobID string) (*Checkpoint, error) {
	path, err := s.checkpointPath(jobID)
	if err != nil {
		return nil, err
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		now := nowUTC()
		return &Checkpoint{
			Version:   checkpointVersion,
			JobID:     jobID,
			CreatedAt: now,
			UpdatedAt: now,
			Nodes:     make(map[string]*NodeRecord),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("parse checkpoint %s: %w", path, err)
	}
	if checkpoint.Nodes == nil {
		checkpoint.Nodes = make(map[string]*NodeRecord)
	}
	return &checkpoint, nil
}

// PutBytes is a convenience wrapper around PutReader.
func (s *Store) PutBytes(ctx context.Context, data []byte, mediaType string) (*Artifact, error) {
	return s.PutReader(ctx, strings.NewReader(string(data)), mediaType)
}

// PutReader writes a complete artifact through staging and only publishes it
// to the content-addressed object store after a checksum and atomic rename.
func (s *Store) PutReader(ctx context.Context, reader io.Reader, mediaType string) (artifact *Artifact, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	temp, err := os.CreateTemp(s.stagingDir, "artifact-*.part")
	if err != nil {
		return nil, fmt.Errorf("create staging file: %w", err)
	}
	tempName := temp.Name()
	completed := false
	defer func() {
		_ = temp.Close()
		if !completed {
			_, _ = s.Quarantine(tempName, "artifact write did not commit")
		}
	}()

	hasher := sha256.New()
	written, copyErr := io.Copy(temp, io.TeeReader(&contextReader{ctx: ctx, r: reader}, hasher))
	syncErr := temp.Sync()
	closeErr := temp.Close()

	if copyErr != nil {
		return nil, fmt.Errorf("write artifact: %w", copyErr)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if syncErr != nil {
		return nil, fmt.Errorf("sync artifact: %w", syncErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close artifact staging file: %w", closeErr)
	}

	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	destination, err := s.objectPath(Artifact{Digest: digest})
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tempName, destination); err != nil {
		return nil, fmt.Errorf("commit artifact: %w", err)
	}
	completed = true
	if err := fsyncDir(s.objectsDir); err != nil {
		return nil, fmt.Errorf("sync object directory: %w", err)
	}

	return &Artifact{Digest: digest, Size: written, MediaType: mediaType}, nil
}
