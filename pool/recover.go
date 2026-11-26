package pool

import (
	"os"
	"path/filepath"
)

// reconcile rebuilds a consistent state after an abrupt shutdown. It works
// purely from the persisted dependency graph and blob directory; no file
// modification times are consulted.
func (p *Pool) reconcile() (RecoveryReport, error) {
	var report RecoveryReport
	now := p.nowFunc()

	// 1. Drop leases whose absolute deadline has already passed. A renderer
	// that crashed can never renew, so its protection must lapse.
	for key, r := range p.state.Records {
		if len(r.Leases) == 0 {
			continue
		}
		kept := r.Leases[:0]
		for _, l := range r.Leases {
			if l.ExpiresAt > now {
				kept = append(kept, l)
			} else {
				report.ExpiredLeases++
			}
		}
		p.state.Records[key].Leases = kept
	}

	// 2. Drop records whose blob is no longer on disk.
	missing := map[string]bool{}
	for key, r := range p.state.Records {
		if _, err := os.Stat(filepath.Join(p.root, r.BlobPath)); err != nil {
			if !os.IsNotExist(err) {
				return report, err
			}
			missing[key] = true
			delete(p.state.Records, key)
			report.MissingBlobs++
		}
	}

	// 3. Keep only edges whose both endpoints exist; every removed edge lowers
	// the reference count of its target on rebuild.
	keptEdges := p.state.Edges[:0]
	for _, e := range p.state.Edges {
		if missing[e.From.String()] || missing[e.To.String()] ||
			!p.hasRecord(e.From) || !p.hasRecord(e.To) {
			report.DanglingEdges++
			continue
		}
		keptEdges = append(keptEdges, e)
	}
	p.state.Edges = keptEdges

	// 4. Reference counts are derived state: recompute them from the graph so
	// they cannot drift from the edges that define them.
	p.rebuildRefCounts()

	// 5. Sweep blobs with no record. This is the crash-mid-Put case: the blob
	// made it to disk but the state rename never happened.
	entries, err := os.ReadDir(p.blobsPath())
	if err != nil {
		return report, err
	}
	liveBlobs := map[string]bool{}
	for _, r := range p.state.Records {
		liveBlobs[filepath.Base(r.BlobPath)] = true
	}
	for _, entry := range entries {
		if entry.IsDir() || liveBlobs[entry.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(p.blobsPath(), entry.Name())); err != nil {
			return report, err
		}
		report.OrphanBlobs++
	}

	if report != (RecoveryReport{}) {
		if err := p.persist(); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (p *Pool) hasRecord(fp Fingerprint) bool {
	_, ok := p.state.Records[fp.String()]
	return ok
}
