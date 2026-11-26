package pool

import (
	"errors"
	"testing"
	"time"
)

func TestLeaseProtectsRetiredProxy(t *testing.T) {
	p, _ := openTestPool(t)
	advance := p.setClockForTest(1_000_000)

	proxy, err := p.Put("asset.bin", []byte("leased-input"))
	if err != nil {
		t.Fatal(err)
	}
	fp := proxy.Fingerprint

	lease, err := p.AcquireLease(fp, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := p.ActiveLeases(fp); err != nil || n != 1 {
		t.Fatalf("expected 1 active lease, got %d (err %v)", n, err)
	}

	if _, err := p.Retire(fp); err != nil {
		t.Fatal(err)
	}
	swept, err := p.Sweep()
	if err != nil {
		t.Fatal(err)
	}
	if len(swept) != 0 {
		t.Fatalf("leased proxy must not be swept, got %v", swept)
	}

	// After the deadline passes the protection lapses and Sweep collects it.
	advance(61)
	swept, err = p.Sweep()
	if err != nil {
		t.Fatal(err)
	}
	if len(swept) != 1 || swept[0] != fp {
		t.Fatalf("expected lease-expired proxy to be swept, got %v", swept)
	}
	if _, _, err := p.Get(fp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after sweep, got %v", err)
	}

	// The lease ID used above is retained for clarity.
	_ = lease.ID
}

func TestRenewAndReleaseLease(t *testing.T) {
	p, _ := openTestPool(t)
	advance := p.setClockForTest(2_000_000)

	proxy, _ := p.Put("x.bin", []byte("renew-me"))
	fp := proxy.Fingerprint
	lease, err := p.AcquireLease(fp, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	advance(20)
	if _, err := p.RenewLease(fp, lease.ID, 30*time.Second); err != nil {
		t.Fatalf("renew valid lease: %v", err)
	}
	advance(25)
	if n, _ := p.ActiveLeases(fp); n != 1 {
		t.Fatalf("renewed lease should still be active, got %d", n)
	}

	if err := p.ReleaseLease(fp, lease.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := p.ActiveLeases(fp); n != 0 {
		t.Fatalf("released lease should be gone, got %d", n)
	}

	if _, err := p.RenewLease(fp, lease.ID, time.Second); !errors.Is(err, ErrLeaseRequired) {
		t.Fatalf("expected ErrLeaseRequired, got %v", err)
	}
}
