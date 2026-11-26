package batch

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// WAL record types. A batch moves strictly forward through this lifecycle.
const (
	recBatchOpen      = "batch_open"
	recFragmentCommit = "fragment_commit"
	recFileCommit     = "file_commit"
	recBatchCommit    = "batch_commit"
)

// ErrCorruptWAL means the transaction log ends with a torn or tampered
// record. All complete records preceding it remain trustworthy.
var ErrCorruptWAL = errors.New("corrupt transaction log record")

type walRecord struct {
	Type     string         `json:"type"`
	BatchID  string         `json:"batch_id,omitempty"`
	SpecHash string         `json:"spec_hash,omitempty"`
	Path     string         `json:"path,omitempty"`
	Index    int            `json:"index,omitempty"`
	Fragment *FragmentEntry `json:"fragment,omitempty"`
	FileHash string         `json:"file_hash,omitempty"`
	Size     int64          `json:"size,omitempty"`
	Manifest string         `json:"manifest_digest,omitempty"`
}

// wal is an append-only, fsync'd transaction log.
type wal struct {
	file *os.File
	path string
}

func openWAL(dir string) (*wal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "txn.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &wal{file: f, path: path}, nil
}

func (w *wal) Close() error { return w.file.Close() }

// append encodes one record as: [4-byte len][4-byte crc32][json payload]
// and fsyncs before returning, so a returned record survives a power loss.
func (w *wal) append(rec walRecord) error {
	payload, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	frame := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(frame[4:8], crc32.ChecksumIEEE(payload))
	copy(frame[8:], payload)

	if _, err := w.file.Write(frame); err != nil {
		return err
	}
	return w.file.Sync()
}

// replayState is the projection reconstructed from a log.
type replayState struct {
	batchID   string
	specHash  string
	files     map[string]*fileState
	order     []string
	committed bool
}

func newReplayState() *replayState {
	return &replayState{files: map[string]*fileState{}}
}

// replay reads every complete record and applies it to a fresh projection.
// A torn tail record (partial write from a crash) is reported via
// truncatedTail so the caller can truncate it on the next open.
func replayWAL(dir string) (state *replayState, validBytes int64, err error) {
	path := filepath.Join(dir, "txn.log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newReplayState(), 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	state = newReplayState()
	var offset int64
	for {
		var header [8]byte
		n, readErr := io.ReadFull(f, header[:])
		if readErr == io.EOF && n == 0 {
			return state, offset, nil
		}
		if readErr == io.ErrUnexpectedEOF || readErr == io.EOF {
			return state, offset, fmt.Errorf("%w: truncated header at %d", ErrCorruptWAL, offset)
		}
		if readErr != nil {
			return nil, 0, readErr
		}
		length := binary.BigEndian.Uint32(header[0:4])
		checksum := binary.BigEndian.Uint32(header[4:8])
		if length > 16*1024*1024 {
			return state, offset, fmt.Errorf("%w: implausible record length %d", ErrCorruptWAL, length)
		}

		payload := make([]byte, length)
		if _, readErr := io.ReadFull(f, payload); readErr != nil {
			return state, offset, fmt.Errorf("%w: truncated payload at %d", ErrCorruptWAL, offset)
		}
		if crc32.ChecksumIEEE(payload) != checksum {
			return nil, 0, fmt.Errorf("%w: checksum mismatch at %d", ErrCorruptWAL, offset)
		}

		var rec walRecord
		if jsonErr := json.Unmarshal(payload, &rec); jsonErr != nil {
			return nil, 0, fmt.Errorf("%w: %v", ErrCorruptWAL, jsonErr)
		}
		applyRecord(state, rec)
		offset += int64(8 + length)
	}
}

func applyRecord(state *replayState, rec walRecord) {
	switch rec.Type {
	case recBatchOpen:
		state.batchID = rec.BatchID
		state.specHash = rec.SpecHash
	case recFragmentCommit:
		fs := state.ensureFile(rec.Path)
		if rec.Fragment != nil {
			fs.fragments[rec.Fragment.Index] = *rec.Fragment
		}
	case recFileCommit:
		fs := state.ensureFile(rec.Path)
		fs.fileHash = rec.FileHash
		fs.size = rec.Size
		fs.committed = true
	case recBatchCommit:
		state.committed = true
	}
}

func (s *replayState) ensureFile(path string) *fileState {
	fs, ok := s.files[path]
	if !ok {
		fs = newFileState()
		s.files[path] = fs
		s.order = append(s.order, path)
	}
	return fs
}

// truncateAt discards a torn tail so the log stays append-only over valid
// records only.
func truncateWALAt(dir string, offset int64) error {
	path := filepath.Join(dir, "txn.log")
	return os.Truncate(path, offset)
}

// FragmentStatus is the externally visible state of one fragment.
type FragmentStatus struct {
	Index  int    `json:"index"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// FileStatus is the externally visible WAL state of one output file.
type FileStatus struct {
	Path           string           `json:"path"`
	Committed      bool             `json:"committed"`
	FileSHA256     string           `json:"file_sha256,omitempty"`
	Size           int64            `json:"size"`
	CommittedFrags []FragmentStatus `json:"committed_fragments"`
}

// WALInspection summarizes a replayed transaction log.
type WALInspection struct {
	BatchID   string       `json:"batch_id"`
	SpecHash  string       `json:"spec_hash"`
	Committed bool         `json:"committed"`
	Files     []FileStatus `json:"files"`
}

// InspectWAL replays the log and returns a stable, JSON-friendly projection.
func InspectWAL(dir string) (WALInspection, int64, error) {
	state, validBytes, err := replayWAL(dir)
	if err != nil && !errors.Is(err, ErrCorruptWAL) {
		return WALInspection{}, 0, err
	}

	inspection := WALInspection{
		BatchID:   state.batchID,
		SpecHash:  state.specHash,
		Committed: state.committed,
	}
	for _, path := range state.order {
		fs := state.files[path]
		status := FileStatus{
			Path:       path,
			Committed:  fs.committed,
			FileSHA256: fs.fileHash,
			Size:       fs.size,
		}
		for index := range fs.fragments {
			entry := fs.fragments[index]
			status.CommittedFrags = append(status.CommittedFrags, FragmentStatus{
				Index:  entry.Index,
				Size:   entry.Size,
				SHA256: entry.SHA256,
			})
		}
		sort.Slice(status.CommittedFrags, func(i, j int) bool {
			return status.CommittedFrags[i].Index < status.CommittedFrags[j].Index
		})
		inspection.Files = append(inspection.Files, status)
	}
	return inspection, validBytes, nil
}
