package pool

import "time"

// AcquireLease grants a lease protecting fp from reclamation for ttl. The
// lease holds an absolute expiry time so it stays meaningful across
// processes and restarts.
func (p *Pool) AcquireLease(fp Fingerprint, ttl time.Duration) (Lease, error) {
	if ttl <= 0 {
		return Lease{}, ErrLeaseRequired
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.recordLocked(fp)
	if !ok {
		return Lease{}, ErrNotFound
	}
	p.pruneLeasesLocked(r)
	lease := Lease{
		ID:        newLeaseID(),
		ExpiresAt: p.nowFunc() + int64(ttl/time.Second),
	}
	r.Leases = append(r.Leases, lease)
	r.UpdatedAt = p.nowFunc()
	if err := p.persist(); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

// RenewLease extends an existing lease by ttl.
func (p *Pool) RenewLease(fp Fingerprint, leaseID string, ttl time.Duration) (Lease, error) {
	if ttl <= 0 {
		return Lease{}, ErrLeaseRequired
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.recordLocked(fp)
	if !ok {
		return Lease{}, ErrNotFound
	}
	for i := range r.Leases {
		if r.Leases[i].ID != leaseID {
			continue
		}
		if r.Leases[i].ExpiresAt <= p.nowFunc() {
			return Lease{}, ErrLeaseRequired
		}
		r.Leases[i].ExpiresAt = p.nowFunc() + int64(ttl/time.Second)
		r.UpdatedAt = p.nowFunc()
		renewed := r.Leases[i]
		if err := p.persist(); err != nil {
			return Lease{}, err
		}
		return renewed, nil
	}
	return Lease{}, ErrLeaseRequired
}

// ReleaseLease drops a lease, making its proxy reclaimable again if nothing
// else protects it. Releasing an unknown lease is a no-op.
func (p *Pool) ReleaseLease(fp Fingerprint, leaseID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	r, ok := p.recordLocked(fp)
	if !ok {
		return ErrNotFound
	}
	kept := r.Leases[:0]
	for _, l := range r.Leases {
		if l.ID != leaseID {
			kept = append(kept, l)
		}
	}
	r.Leases = kept
	r.UpdatedAt = p.nowFunc()
	return p.persist()
}

// pruneLeasesLocked removes already-expired leases in place.
func (p *Pool) pruneLeasesLocked(r *record) {
	now := p.nowFunc()
	kept := r.Leases[:0]
	for _, l := range r.Leases {
		if l.ExpiresAt > now {
			kept = append(kept, l)
		}
	}
	r.Leases = kept
}
