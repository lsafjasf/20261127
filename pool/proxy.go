package pool

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// Put stores a proxy produced from input data under the caller's logical file
// name. The returned proxy's identity is the input fingerprint: the same
// content put twice is stored once, while the same name with different
// content becomes a distinct proxy. Modification time never participates in
// identity or replacement decisions.
func (p *Pool) Put(name string, data []byte) (Proxy, error) {
	fp := FingerprintBytes(data)

	p.mu.Lock()
	defer p.mu.Unlock()

	if r, ok := p.recordLocked(fp); ok {
		return toProxy(r, p.ref[fp.String()], p.activeLeasesLocked(r)), nil
	}

	relBlob := filepath.Join(blobsDir, fp.String())
	if err := p.writeBlob(fp, data); err != nil {
		return Proxy{}, err
	}
	r := &record{
		LogicalFileName: name,
		Fingerprint:     fp,
		BlobPath:        relBlob,
		Size:            int64(len(data)),
		State:           StateLive,
		UpdatedAt:       p.nowFunc(),
	}
	p.state.Records[fp.String()] = r
	if err := p.persist(); err != nil {
		_ = os.Remove(filepath.Join(p.root, relBlob))
		return Proxy{}, err
	}
	return toProxy(r, 0, 0), nil
}

// writeBlob stages the blob under a temporary name and renames it into place
// so a crash can never leave a partially written blob under its final name.
func (p *Pool) writeBlob(fp Fingerprint, data []byte) error {
	final := filepath.Join(p.blobsPath(), fp.String())
	tmp, err := os.CreateTemp(p.blobsPath(), "tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// Get returns the proxy's raw content and a metadata snapshot.
func (p *Pool) Get(fp Fingerprint) ([]byte, Proxy, error) {
	p.mu.RLock()
	r, ok := p.recordLocked(fp)
	if !ok {
		p.mu.RUnlock()
		return nil, Proxy{}, ErrNotFound
	}
	snapshot := toProxy(r, p.ref[fp.String()], p.activeLeasesLocked(r))
	blob := filepath.Join(p.root, r.BlobPath)
	p.mu.RUnlock()
	data, err := os.ReadFile(blob)
	if err != nil {
		return nil, Proxy{}, err
	}
	return data, snapshot, nil
}

// Open opens the proxy blob for reading. The caller must close the file.
func (p *Pool) Open(fp Fingerprint) (*os.File, Proxy, error) {
	p.mu.RLock()
	r, ok := p.recordLocked(fp)
	if !ok {
		p.mu.RUnlock()
		return nil, Proxy{}, ErrNotFound
	}
	snapshot := toProxy(r, p.ref[fp.String()], p.activeLeasesLocked(r))
	blob := filepath.Join(p.root, r.BlobPath)
	p.mu.RUnlock()
	f, err := os.Open(blob)
	if err != nil {
		return nil, Proxy{}, err
	}
	return f, snapshot, nil
}

// RefCount returns the number of other proxies that reference fp.
func (p *Pool) RefCount(fp Fingerprint) (int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if _, ok := p.recordLocked(fp); !ok {
		return 0, ErrNotFound
	}
	return p.ref[fp.String()], nil
}

// ActiveLeases returns the number of currently non-expired leases on fp.
func (p *Pool) ActiveLeases(fp Fingerprint) (int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.recordLocked(fp)
	if !ok {
		return 0, ErrNotFound
	}
	return p.activeLeasesLocked(r), nil
}

func (p *Pool) activeLeasesLocked(r *record) int {
	now := p.nowFunc()
	n := 0
	for _, l := range r.Leases {
		if l.ExpiresAt > now {
			n++
		}
	}
	return n
}

func newLeaseID() string {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func containsLease(leases []Lease, id string) bool {
	for _, l := range leases {
		if l.ID == id {
			return true
		}
	}
	return false
}
