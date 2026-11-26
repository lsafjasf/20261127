package pool

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestPool(t *testing.T) (*Pool, string) {
	t.Helper()
	dir := t.TempDir()
	p, _, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p, dir
}

func TestPutSameNameDifferentVersions(t *testing.T) {
	p, dir := openTestPool(t)

	v1, err := p.Put("scene/color.dat", []byte("input-v1"))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := p.Put("scene/color.dat", []byte("input-v2"))
	if err != nil {
		t.Fatal(err)
	}
	if v1.Fingerprint == v2.Fingerprint {
		t.Fatal("same file name with different input must yield different fingerprints")
	}

	// Both blobs coexist on disk; reclaiming one can never touch the other.
	if _, err := os.Stat(filepath.Join(dir, "blobs", v1.Fingerprint.String())); err != nil {
		t.Fatalf("v1 blob missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "blobs", v2.Fingerprint.String())); err != nil {
		t.Fatalf("v2 blob missing: %v", err)
	}

	// Identical content is deduplicated regardless of the logical name.
	again, err := p.Put("a/completely/different/name.dat", []byte("input-v1"))
	if err != nil {
		t.Fatal(err)
	}
	if again.Fingerprint != v1.Fingerprint {
		t.Fatal("identical input must map to one fingerprint")
	}
	if p.Stat().Total != 2 {
		t.Fatalf("expected 2 proxies, got %d", p.Stat().Total)
	}
}

func TestIdentityIgnoresModificationTime(t *testing.T) {
	p, _ := openTestPool(t)

	first, err := p.Put("f.dat", []byte("same-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	// Advance the fake clock and rewrite with identical bytes. The blob path
	// is derived from the content hash, so time can never change identity.
	blob := filepath.Join(p.root, blobsDir, first.Fingerprint.String())
	if err := os.Chtimes(blob, time.Unix(1_000_000_000, 0), time.Unix(1_000_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	second, err := p.Put("f.dat", []byte("same-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Fingerprint != first.Fingerprint {
		t.Fatal("fingerprint must depend on content only, not mtime")
	}
}

func TestGetMissing(t *testing.T) {
	p, _ := openTestPool(t)
	if _, _, err := p.Get(FingerprintBytes([]byte("absent"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
