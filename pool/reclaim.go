package pool

import (
	"os"
	"path/filepath"
)

// Retire marks a proxy as a reclamation candidate. Retired proxies remain
// readable and are only deleted by Sweep once every reference and active
// lease has gone. Retiring an already retired proxy is a no-op.
func (p *Pool) Retire(fp Fingerprint) (Proxy, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.recordLocked(fp)
	if !ok {
		return Proxy{}, ErrNotFound
	}
	if r.State != StateRetired {
		r.State = StateRetired
		r.UpdatedAt = p.nowFunc()
		if err := p.persist(); err != nil {
			return Proxy{}, err
		}
	}
	return toProxy(r, p.ref[fp.String()], p.activeLeasesLocked(r)), nil
}

// Reactivate moves a retired proxy back to the live state.
func (p *Pool) Reactivate(fp Fingerprint) (Proxy, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.recordLocked(fp)
	if !ok {
		return Proxy{}, ErrNotFound
	}
	if r.State != StateLive {
		r.State = StateLive
		r.UpdatedAt = p.nowFunc()
		if err := p.persist(); err != nil {
			return Proxy{}, err
		}
	}
	return toProxy(r, p.ref[fp.String()], p.activeLeasesLocked(r)), nil
}

// Sweep deletes every proxy that is simultaneously retired, unreferenced and
// free of active leases. Deletions cascade: removing a proxy drops its
// outgoing edges, which may make other retired proxies unreferenced, and
// those are collected in the same run. It returns the fingerprints removed.
func (p *Pool) Sweep() ([]Fingerprint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var swept []Fingerprint
	for {
		candidate, ok := p.nextSweepCandidateLocked()
		if !ok {
			break
		}
		if err := p.deleteRecordLocked(candidate); err != nil {
			return swept, err
		}
		swept = append(swept, candidate)
	}
	if len(swept) > 0 {
		if err := p.persist(); err != nil {
			return swept, err
		}
	}
	return swept, nil
}

func (p *Pool) nextSweepCandidateLocked() (Fingerprint, bool) {
	now := p.nowFunc()
	for key, r := range p.state.Records {
		kept := r.Leases[:0]
		for _, l := range r.Leases {
			if l.ExpiresAt > now {
				kept = append(kept, l)
			}
		}
		r.Leases = kept
		if r.State == StateRetired && p.ref[key] == 0 && len(r.Leases) == 0 {
			fp, err := ParseFingerprint(key)
			if err != nil {
				continue
			}
			return fp, true
		}
	}
	return Fingerprint{}, false
}

// deleteRecordLocked removes the blob, the record and every edge touching it.
// Reference counts of the proxy's former dependencies are decremented so the
// fixpoint loop in Sweep can cascade.
func (p *Pool) deleteRecordLocked(fp Fingerprint) error {
	key := fp.String()
	r, ok := p.state.Records[key]
	if !ok {
		return ErrNotFound
	}
	if err := os.Remove(filepath.Join(p.root, r.BlobPath)); err != nil && !os.IsNotExist(err) {
		return err
	}

	kept := p.state.Edges[:0]
	for _, e := range p.state.Edges {
		switch {
		case e.From == fp:
			// fp used to protect its dependency.
			p.ref[e.To.String()]--
			if p.ref[e.To.String()] <= 0 {
				delete(p.ref, e.To.String())
			}
		case e.To == fp:
			// incoming edge disappears with the target
		default:
			kept = append(kept, e)
		}
	}
	p.state.Edges = kept
	delete(p.state.Records, key)
	delete(p.ref, key)
	return nil
}

// List returns snapshots of all proxies in the pool.
func (p *Pool) List() []Proxy {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]Proxy, 0, len(p.state.Records))
	for _, r := range p.state.Records {
		out = append(out, toProxy(r, p.ref[r.Fingerprint.String()], p.activeLeasesLocked(r)))
	}
	return out
}

// Stats summarizes pool contents.
type Stats struct {
	Total   int
	Live    int
	Retired int
	Edges   int
}

// Stat returns aggregate pool statistics.
func (p *Pool) Stat() Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var s Stats
	s.Edges = len(p.state.Edges)
	s.Total = len(p.state.Records)
	for _, r := range p.state.Records {
		if r.State == StateRetired {
			s.Retired++
		} else {
			s.Live++
		}
	}
	return s
}
