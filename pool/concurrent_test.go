package pool

import (
	"sync"
	"testing"
	"time"
)

func TestConcurrentOperations(t *testing.T) {
	p, _ := openTestPool(t)

	proxies := make([]Fingerprint, 8)
	for i := range proxies {
		proxy, err := p.Put("shared.dat", []byte{byte(i)})
		if err != nil {
			t.Fatal(err)
		}
		proxies[i] = proxy.Fingerprint
	}

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for round := 0; round < 25; round++ {
				fp := proxies[(id+round)%len(proxies)]
				lease, err := p.AcquireLease(fp, time.Hour)
				if err != nil {
					t.Errorf("acquire: %v", err)
					return
				}
				if round%2 == 0 {
					if err := p.AddReference(proxies[id], fp); err != nil {
						t.Errorf("add ref: %v", err)
						return
					}
				}
				if _, err := p.RenewLease(fp, lease.ID, time.Hour); err != nil {
					t.Errorf("renew: %v", err)
					return
				}
				if err := p.ReleaseLease(fp, lease.ID); err != nil {
					t.Errorf("release: %v", err)
					return
				}
				if round%2 == 0 {
					if err := p.RemoveReference(proxies[id], fp); err != nil {
						t.Errorf("remove ref: %v", err)
						return
					}
				}
			}
		}(worker)
	}
	wg.Wait()

	// All references are removed again, and all leases were released:
	// refcounts and lease counts must have returned to zero.
	for _, fp := range proxies {
		if rc, err := p.RefCount(fp); err != nil || rc != 0 {
			t.Fatalf("fp %s refcount=%d err=%v, want 0", fp, rc, err)
		}
		if n, err := p.ActiveLeases(fp); err != nil || n != 0 {
			t.Fatalf("fp %s leases=%d err=%v, want 0", fp, n, err)
		}
	}

	// Every proxy is retired and must be collectable without touching blobs
	// that still have references in the same run.
	for _, fp := range proxies {
		if _, err := p.Retire(fp); err != nil {
			t.Fatal(err)
		}
	}
	swept, err := p.Sweep()
	if err != nil {
		t.Fatal(err)
	}
	if len(swept) != len(proxies) {
		t.Fatalf("expected %d swept, got %d", len(proxies), len(swept))
	}
}
