package pool

import "errors"

var (
	// ErrNotFound means no proxy exists for the given fingerprint.
	ErrNotFound = errors.New("pool: proxy not found")
	// ErrBadFingerprint means a fingerprint could not be decoded.
	ErrBadFingerprint = errors.New("pool: malformed fingerprint")
	// ErrDependencyMissing means an edge references a proxy that is absent.
	ErrDependencyMissing = errors.New("pool: referenced dependency is not in the pool")
	// ErrLocked means another process already holds the pool lock.
	ErrLocked = errors.New("pool: pool is locked by another process")
	// ErrLeaseRequired means the operation needs a valid lease that was not supplied.
	ErrLeaseRequired = errors.New("pool: missing or invalid lease")
)
