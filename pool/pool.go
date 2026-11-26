package pool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	stateVersion = 1
	stateName    = "state.json"
	tempState    = "state.json.tmp"
	blobsDir     = "blobs"
)

// RecoveryReport describes repairs performed while opening a pool. These are
// the fixes that make a pool consistent again after a renderer crash.
type RecoveryReport struct {
	// ExpiredLeases is the number of leases dropped because their deadline passed.
	ExpiredLeases int
	// OrphanBlobs is the number of blob files present on disk but referenced by
	// no record (typical of a crash mid-Put, after the blob was written but
	// before state was committed).
	OrphanBlobs int
	// MissingBlobs is the number of records whose blob was absent on disk.
	// Those records and their edges are removed.
	MissingBlobs int
	// DanglingEdges is the number of edges whose endpoint record was missing.
	DanglingEdges int
}

// Pool is a safe-for-concurrent-use render proxy store.
type Pool struct {
	root string
	lock *fileLock

	mu      sync.RWMutex
	state   *stateFile
	ref     map[string]int // incoming edge count keyed by target fingerprint hex
	nowFunc func() int64
}

// Open opens (creating if needed) the pool rooted at dir and runs crash
// recovery on its contents. Only one Pool per directory may be open across
// all processes.
func Open(dir string) (*Pool, RecoveryReport, error) {
	return openWithClock(dir, func() int64 { return time.Now().Unix() })
}

// openWithClock is Open with an injectable clock, used by tests to make
// lease-expiry decisions during recovery deterministic.
func openWithClock(dir string, nowFunc func() int64) (*Pool, RecoveryReport, error) {
	if err := os.MkdirAll(filepath.Join(dir, blobsDir), 0o755); err != nil {
		return nil, RecoveryReport{}, err
	}
	lock, err := acquireFileLock(dir)
	if err != nil {
		return nil, RecoveryReport{}, err
	}

	p := &Pool{
		root:    dir,
		lock:    lock,
		nowFunc: nowFunc,
	}
	st, err := p.loadState()
	if err != nil {
		_ = lock.release()
		return nil, RecoveryReport{}, err
	}
	p.state = st
	report, err := p.reconcile()
	if err != nil {
		_ = lock.release()
		return nil, RecoveryReport{}, err
	}
	return p, report, nil
}

// Close releases the inter-process lock. In-memory state is already durable
// after every mutating call, so no flushing is required here.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lock == nil {
		return nil
	}
	err := p.lock.release()
	p.lock = nil
	return err
}

func (p *Pool) statePath() string { return filepath.Join(p.root, stateName) }
func (p *Pool) tempPath() string  { return filepath.Join(p.root, tempState) }
func (p *Pool) blobsPath() string { return filepath.Join(p.root, blobsDir) }

func (p *Pool) loadState() (*stateFile, error) {
	data, err := os.ReadFile(p.statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &stateFile{Version: stateVersion, Records: map[string]*record{}}, nil
		}
		return nil, err
	}
	var st stateFile
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Version != stateVersion {
		return nil, os.ErrInvalid
	}
	if st.Records == nil {
		st.Records = map[string]*record{}
	}
	return &st, nil
}

// persist atomically replaces the state file. The temp-file-plus-rename
// sequence guarantees a crash leaves either the old state or the new one,
// never a half-written JSON document.
func (p *Pool) persist() error {
	st := p.state
	st.Edges = append([]edge(nil), st.Edges...)
	sort.SliceStable(st.Edges, func(i, j int) bool { return st.Edges[i].key() < st.Edges[j].key() })

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.tempPath()
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.statePath()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (p *Pool) rebuildRefCounts() {
	p.ref = make(map[string]int, len(p.state.Records))
	for _, e := range p.state.Edges {
		p.ref[e.To.String()]++
	}
}

func (p *Pool) recordLocked(fp Fingerprint) (*record, bool) {
	r, ok := p.state.Records[fp.String()]
	return r, ok
}
