// Package pool manages a render-farm cache of render proxies.
//
// Proxies are addressed by a fingerprint of their input bytes instead of by
// file name or modification time. The pool keeps a persistent dependency
// graph (reference counts are derived from it), per-proxy leases, and a
// two-stage retirement state so that proxies which are still referenced can
// never be reclaimed.
package pool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// State is the reclamation state of a proxy.
type State string

const (
	// StateLive proxies are available for normal use.
	StateLive State = "live"
	// StateRetired proxies are reclamation candidates: they are kept until
	// no client references or active leases point at them.
	StateRetired State = "retired"
)

// Fingerprint is the SHA-256 digest of the input bytes that produced a proxy.
// Two input versions sharing the same file name have different fingerprints
// and therefore live in separate blobs.
type Fingerprint [sha256.Size]byte

// FingerprintBytes computes the fingerprint of raw input bytes.
func FingerprintBytes(data []byte) Fingerprint {
	return sha256.Sum256(data)
}

// ParseFingerprint decodes a hex-encoded fingerprint.
func ParseFingerprint(s string) (Fingerprint, error) {
	var fp Fingerprint
	raw, err := hex.DecodeString(s)
	if err != nil {
		return fp, err
	}
	if len(raw) != sha256.Size {
		return fp, ErrBadFingerprint
	}
	copy(fp[:], raw)
	return fp, nil
}

// String renders the fingerprint as lowercase hex.
func (f Fingerprint) String() string {
	return hex.EncodeToString(f[:])
}

// MarshalJSON encodes the fingerprint as a hex string.
func (f Fingerprint) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.String())
}

// UnmarshalJSON decodes a hex-encoded fingerprint.
func (f *Fingerprint) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseFingerprint(s)
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// Lease grants one client temporary protection against reclamation.
type Lease struct {
	ID        string `json:"id"`
	ExpiresAt int64  `json:"expires_at"` // unix seconds, absolute deadline
}

// record is the on-disk metadata of one proxy.
type record struct {
	// LogicalFileName is the name requested by the caller. It is retained for
	// diagnostics only: identity comes from Fingerprint, never this field.
	LogicalFileName string      `json:"logical_file_name"`
	Fingerprint     Fingerprint `json:"fingerprint"`
	BlobPath        string      `json:"blob_path"` // relative to the pool root
	Size            int64       `json:"size"`
	State           State       `json:"state"`
	Leases          []Lease     `json:"leases,omitempty"`
	UpdatedAt       int64       `json:"updated_at"` // unix seconds
}

// edge records that proxy From references proxy To (From depends on To).
// The reference count of To is the number of distinct incoming edges.
type edge struct {
	From Fingerprint `json:"from"`
	To   Fingerprint `json:"to"`
}

func (e edge) key() string {
	return e.From.String() + "->" + e.To.String()
}

// stateFile is the complete persisted state of the pool.
type stateFile struct {
	Version int                `json:"version"`
	Records map[string]*record `json:"records"`
	Edges   []edge             `json:"edges"`
}

// Proxy is an immutable snapshot of proxy metadata handed to callers.
type Proxy struct {
	LogicalFileName string
	Fingerprint     Fingerprint
	Size            int64
	State           State
	RefCount        int
	ActiveLeases    int
}

func toProxy(r *record, refCount, activeLeases int) Proxy {
	return Proxy{
		LogicalFileName: r.LogicalFileName,
		Fingerprint:     r.Fingerprint,
		Size:            r.Size,
		State:           r.State,
		RefCount:        refCount,
		ActiveLeases:    activeLeases,
	}
}
