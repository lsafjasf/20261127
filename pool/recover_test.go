package pool

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fakeNow int64 = 1_700_000_000

func TestRecoveryRebuildsRefCountsAndLeases(t *testing.T) {
	dir := t.TempDir()

	p, _, err := openWithClock(dir, func() int64 { return fakeNow })
	if err != nil {
		t.Fatal(err)
	}
	advance := p.setClockForTest(fakeNow)
	dep, _ := p.Put("dep", []byte("dep-bytes"))
	parent, _ := p.Put("parent", []byte("parent-bytes"))
	if err := p.AddReference(parent.Fingerprint, dep.Fingerprint); err != nil {
		t.Fatal(err)
	}
	lease, err := p.AcquireLease(dep.Fingerprint, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// A second, shorter lease on another proxy.
	advance(30)
	expired, err := p.AcquireLease(parent.Fingerprint, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = lease
	_ = expired
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate restart after a crash at fakeNow+65: the parent's lease
	// (acquired at +30 for 30s) is past its deadline, the dep's 60s lease
	// (acquired at +0) is still valid for another 5s.
	nowAfter := advance(35)
	reopened, report, err := openWithClock(dir, func() int64 { return nowAfter })
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	if report.ExpiredLeases != 1 {
		t.Fatalf("expected 1 expired lease pruned, got %+v", report)
	}
	if rc, _ := reopened.RefCount(dep.Fingerprint); rc != 1 {
		t.Fatalf("refcount must be rebuilt from graph, got %d", rc)
	}
	if n, _ := reopened.ActiveLeases(dep.Fingerprint); n != 1 {
		t.Fatalf("valid lease must survive restart, got %d", n)
	}
	if n, _ := reopened.ActiveLeases(parent.Fingerprint); n != 0 {
		t.Fatalf("expired lease must be pruned, got %d", n)
	}

	// Retiring the referenced proxy still cannot reclaim it: graph-derived
	// protection works with no in-memory history beyond the restart.
	if _, err := reopened.Retire(dep.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if swept, _ := reopened.Sweep(); len(swept) != 0 {
		t.Fatalf("rebuilt graph must protect referenced proxy, got %v", swept)
	}
}

func TestRecoveryCleansMidPutOrphanBlob(t *testing.T) {
	dir := t.TempDir()
	p, _, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	proxy, _ := p.Put("good", []byte("good-bytes"))

	// Crash between blob write and state commit: an untracked blob exists.
	orphan := FingerprintBytes([]byte("committed-blob-only"))
	if err := os.WriteFile(filepath.Join(dir, blobsDir, orphan.String()), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, report, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if report.OrphanBlobs != 1 {
		t.Fatalf("expected 1 orphan blob, got %+v", report)
	}
	if _, err := os.Stat(filepath.Join(dir, blobsDir, orphan.String())); !os.IsNotExist(err) {
		t.Fatalf("orphan blob should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, blobsDir, proxy.Fingerprint.String())); err != nil {
		t.Fatalf("tracked blob must remain: %v", err)
	}
}

func TestRecoveryDropsRecordWithMissingBlob(t *testing.T) {
	dir := t.TempDir()
	p, _, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := p.Put("parent", []byte("p-bytes"))
	dep, _ := p.Put("dep", []byte("d-bytes"))
	if err := p.AddReference(parent.Fingerprint, dep.Fingerprint); err != nil {
		t.Fatal(err)
	}
	// Simulate external blob loss (disk failure / manual deletion).
	if err := os.Remove(filepath.Join(dir, blobsDir, dep.Fingerprint.String())); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, report, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if report.MissingBlobs != 1 || report.DanglingEdges != 1 {
		t.Fatalf("unexpected report %+v", report)
	}
	if _, _, err := reopened.Get(dep.Fingerprint); !os.IsNotExist(err) && err != ErrNotFound {
		t.Fatalf("dep record should be gone, got %v", err)
	}
	if deps, _ := reopened.Dependencies(parent.Fingerprint); len(deps) != 0 {
		t.Fatalf("dangling edge must be removed, got %v", deps)
	}
}
