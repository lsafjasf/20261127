package batch

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// processFiles walks every spec file, rendering only fragments missing from
// the recovered projection, then assembles and commits each file.
func (e *Engine) processFiles(spec Spec, state *replayState, log *wal) ([]FileEntry, int, error) {
	entries := make([]FileEntry, 0, len(spec.Files))
	rendered := 0

	for _, file := range spec.Files {
		if err := ValidatePath(file.Path); err != nil {
			return nil, rendered, err
		}
		fs := state.ensureFile(file.Path)

		// A previously committed file is fully verified and skipped.
		if fs.committed {
			entries = append(entries, e.committedEntry(file, fs))
			continue
		}

		// Phase 1+2: stage and log each missing fragment independently.
		for index := 0; index < file.Fragments; index++ {
			if _, ok := fs.fragments[index]; ok {
				continue // already durable from an earlier attempt
			}
			didRender, err := e.renderAndCommitFragment(file, index, log, fs)
			if err != nil {
				return nil, rendered, err
			}
			if didRender {
				rendered++
			}
		}

		// Phase 3: assemble fragments into the final file atomically.
		fileHash, size, err := e.assembleFile(file, fs, log)
		if err != nil {
			return nil, rendered, err
		}

		entries = append(entries, FileEntry{
			Path:      file.Path,
			Size:      size,
			SHA256:    fileHash,
			Fragments: orderedFragments(fs),
		})
	}
	return entries, rendered, nil
}

// renderAndCommitFragment runs the per-fragment staged commit:
// render -> temp file -> fsync -> atomic rename into stage -> WAL record.
func (e *Engine) renderAndCommitFragment(file FileSpec, index int, log *wal, fs *fileState) (bool, error) {
	length := FragmentLength(file, index)
	data, err := e.cfg.Renderer.Render(file, index, length)
	if err != nil {
		return false, err
	}
	if len(data) != length {
		return false, fmt.Errorf("renderer returned %d bytes, expected %d", len(data), length)
	}

	fragPath := e.fragmentPath(file.Path, index)
	if err := writeAtomic(fragPath, data, 0o644); err != nil {
		return false, err
	}
	if err := e.hook(HookAfterFragmentStage, file, index); err != nil {
		return false, err
	}

	entry := FragmentEntry{Index: index, Size: int64(len(data)), SHA256: sha256Hex(data)}
	if err := log.append(walRecord{
		Type:     recFragmentCommit,
		Path:     file.Path,
		Index:    index,
		Fragment: &entry,
	}); err != nil {
		return false, err
	}
	fs.fragments[index] = entry
	if err := e.hook(HookAfterFragmentWAL, file, index); err != nil {
		return false, err
	}
	return true, nil
}

// assembleFile concatenates staged fragments, verifies the whole-file hash,
// publishes the result with an atomic rename and records file_commit.
func (e *Engine) assembleFile(file FileSpec, fs *fileState, log *wal) (string, int64, error) {
	hasher := sha256.New()
	var size int64

	tmp, err := os.CreateTemp(e.stageDir(), ".assemble-*")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	for index := 0; index < file.Fragments; index++ {
		entry, ok := fs.fragments[index]
		if !ok {
			_ = tmp.Close()
			return "", 0, fmt.Errorf("missing committed fragment %d of %s", index, file.Path)
		}
		frag, err := os.Open(e.fragmentPath(file.Path, index))
		if err != nil {
			_ = tmp.Close()
			return "", 0, err
		}
		stat, err := frag.Stat()
		if err != nil {
			_ = frag.Close()
			_ = tmp.Close()
			return "", 0, err
		}
		if stat.Size() != entry.Size {
			_ = frag.Close()
			_ = tmp.Close()
			return "", 0, fmt.Errorf("fragment %d of %s size mismatch on disk", index, file.Path)
		}
		n, err := io.Copy(io.MultiWriter(tmp, hasher), frag)
		_ = frag.Close()
		if err != nil {
			_ = tmp.Close()
			return "", 0, err
		}
		size += n
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}

	fileHash := hex.EncodeToString(hasher.Sum(nil))
	if err := e.hook(HookAfterAssemble, file, -1); err != nil {
		return "", 0, err
	}

	finalPath := filepath.Join(e.filesDir(), file.Path)
	if err := copyFileAtomic(tmpName, finalPath, 0o644); err != nil {
		return "", 0, err
	}

	if err := log.append(walRecord{
		Type:     recFileCommit,
		Path:     file.Path,
		FileHash: fileHash,
		Size:     size,
	}); err != nil {
		return "", 0, err
	}
	fs.fileHash = fileHash
	fs.size = size
	fs.committed = true
	if err := e.hook(HookAfterFileWAL, file, -1); err != nil {
		return "", 0, err
	}
	return fileHash, size, nil
}

func (e *Engine) fragmentPath(filePath string, index int) string {
	safe := strings.ReplaceAll(sanitizePath(filePath), string(os.PathSeparator), "__")
	return filepath.Join(e.stageDir(), safe+fmt.Sprintf(".frag%04d", index))
}

func (e *Engine) committedEntry(file FileSpec, fs *fileState) FileEntry {
	entry := FileEntry{
		Path:      file.Path,
		Size:      fs.size,
		SHA256:    fs.fileHash,
		Fragments: orderedFragments(fs),
	}
	return entry
}

func orderedFragments(fs *fileState) []FragmentEntry {
	indexes := make([]int, 0, len(fs.fragments))
	for index := range fs.fragments {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]FragmentEntry, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, fs.fragments[index])
	}
	return out
}

// ErrUnsafePath rejects path traversal attempts in spec entries.
var ErrUnsafePath = errors.New("unsafe or absolute output path in spec")

// ValidatePath ensures a spec output path stays inside the files directory.
func ValidatePath(path string) error {
	clean := filepath.Clean(path)
	if path == "" || filepath.IsAbs(path) || clean == ".." ||
		(len(clean) >= 3 && clean[:3] == "../") {
		return fmt.Errorf("%w: %q", ErrUnsafePath, path)
	}
	return nil
}

func sanitizePath(path string) string { return filepath.Clean(path) }
