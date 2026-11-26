package pool

// AddReference records that proxy from depends on proxy to. The target's
// reference count is incremented exactly once even if the edge is added
// repeatedly. A reference makes the target ineligible for reclamation.
func (p *Pool) AddReference(from, to Fingerprint) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.recordLocked(from); !ok {
		return ErrNotFound
	}
	if _, ok := p.recordLocked(to); !ok {
		return ErrDependencyMissing
	}
	e := edge{From: from, To: to}
	for _, existing := range p.state.Edges {
		if existing == e {
			return nil
		}
	}
	p.state.Edges = append(p.state.Edges, e)
	p.ref[to.String()]++
	return p.persist()
}

// RemoveReference deletes a dependency edge, lowering the target's reference
// count. Removing an unknown edge is a no-op.
func (p *Pool) RemoveReference(from, to Fingerprint) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	e := edge{From: from, To: to}
	kept := p.state.Edges[:0]
	removed := false
	for _, existing := range p.state.Edges {
		if !removed && existing == e {
			removed = true
			continue
		}
		kept = append(kept, existing)
	}
	p.state.Edges = kept
	if removed {
		p.ref[to.String()]--
		if p.ref[to.String()] <= 0 {
			delete(p.ref, to.String())
		}
		return p.persist()
	}
	return nil
}

// Referrers returns the fingerprints of proxies that reference fp.
func (p *Pool) Referrers(fp Fingerprint) ([]Fingerprint, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if _, ok := p.recordLocked(fp); !ok {
		return nil, ErrNotFound
	}
	var out []Fingerprint
	for _, e := range p.state.Edges {
		if e.To == fp {
			out = append(out, e.From)
		}
	}
	return out, nil
}

// Dependencies returns the fingerprints of proxies that fp references.
func (p *Pool) Dependencies(fp Fingerprint) ([]Fingerprint, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if _, ok := p.recordLocked(fp); !ok {
		return nil, ErrNotFound
	}
	var out []Fingerprint
	for _, e := range p.state.Edges {
		if e.From == fp {
			out = append(out, e.To)
		}
	}
	return out, nil
}
