package pool

import (
	"errors"
	"testing"
)

func TestReferenceCountBlocksReclamation(t *testing.T) {
	p, _ := openTestPool(t)

	dep, _ := p.Put("texture.exr", []byte("texture-bytes"))
	parent, _ := p.Put("scene.usd", []byte("scene-bytes"))

	if err := p.AddReference(parent.Fingerprint, dep.Fingerprint); err != nil {
		t.Fatal(err)
	}
	// Idempotent: duplicate edges count once.
	if err := p.AddReference(parent.Fingerprint, dep.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if rc, _ := p.RefCount(dep.Fingerprint); rc != 1 {
		t.Fatalf("expected refcount 1, got %d", rc)
	}

	if _, err := p.Retire(dep.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if swept, _ := p.Sweep(); len(swept) != 0 {
		t.Fatalf("referenced proxy must survive sweep, got %v", swept)
	}

	referrers, err := p.Referrers(dep.Fingerprint)
	if err != nil || len(referrers) != 1 || referrers[0] != parent.Fingerprint {
		t.Fatalf("unexpected referrers %v err %v", referrers, err)
	}

	if err := p.RemoveReference(parent.Fingerprint, dep.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if rc, _ := p.RefCount(dep.Fingerprint); rc != 0 {
		t.Fatalf("expected refcount 0 after edge removal, got %d", rc)
	}
	swept, err := p.Sweep()
	if err != nil || len(swept) != 1 {
		t.Fatalf("expected sweep of unreferenced proxy, got %v err %v", swept, err)
	}
}

func TestSweepCascadesThroughGraph(t *testing.T) {
	p, _ := openTestPool(t)

	leaf, _ := p.Put("leaf", []byte("leaf-bytes"))
	mid, _ := p.Put("mid", []byte("mid-bytes"))
	root, _ := p.Put("root", []byte("root-bytes"))
	if err := p.AddReference(mid.Fingerprint, leaf.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if err := p.AddReference(root.Fingerprint, mid.Fingerprint); err != nil {
		t.Fatal(err)
	}

	// All retired: root is unreferenced so it goes first; its outgoing edge
	// release makes mid collectable, then leaf.
	for _, fp := range []Fingerprint{leaf.Fingerprint, mid.Fingerprint, root.Fingerprint} {
		if _, err := p.Retire(fp); err != nil {
			t.Fatal(err)
		}
	}
	swept, err := p.Sweep()
	if err != nil {
		t.Fatal(err)
	}
	if len(swept) != 3 {
		t.Fatalf("expected cascade to remove 3 proxies, got %d (%v)", len(swept), swept)
	}
	if p.Stat().Total != 0 || p.Stat().Edges != 0 {
		t.Fatalf("pool should be empty, got %+v", p.Stat())
	}
}

func TestAddReferenceRejectsMissingDependency(t *testing.T) {
	p, _ := openTestPool(t)
	parent, _ := p.Put("p", []byte("p"))
	ghost := FingerprintBytes([]byte("never-stored"))
	if err := p.AddReference(parent.Fingerprint, ghost); !errors.Is(err, ErrDependencyMissing) {
		t.Fatalf("expected ErrDependencyMissing, got %v", err)
	}
}

func TestReactivateKeepsProxy(t *testing.T) {
	p, _ := openTestPool(t)
	proxy, _ := p.Put("r", []byte("r-bytes"))
	if _, err := p.Retire(proxy.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Reactivate(proxy.Fingerprint); err != nil {
		t.Fatal(err)
	}
	swept, _ := p.Sweep()
	if len(swept) != 0 {
		t.Fatalf("live proxy must not be swept, got %v", swept)
	}
}
