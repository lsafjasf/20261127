package evidence_test

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mediafp/internal/evidence"
)

func genKeys(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func appendN(t *testing.T, path string, n int, priv ed25519.PrivateKey) {
	t.Helper()
	for i := 0; i < n; i++ {
		_, err := evidence.Append(path, &evidence.Entry{
			Action:     "compare",
			Subjects:   []string{"a.mp4", "b.mp4"},
			FileHashes: []string{"aaa", "bbb"},
			PayloadFPs: []string{"ppp", "ppp"},
			Verdict:    "REMUX",
			Detail:     json.RawMessage(`{"i":` + strings.Repeat("1", i+1) + `}`),
		}, priv)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestChainVerifyOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chain.jsonl")
	pub, priv := genKeys(t)
	appendN(t, path, 3, priv)

	entries, err := evidence.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("条目数 = %d", len(entries))
	}
	if err := evidence.Verify(entries, pub); err != nil {
		t.Fatalf("校验应通过: %v", err)
	}
}

func TestChainTamperDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chain.jsonl")
	pub, priv := genKeys(t)
	appendN(t, path, 3, priv)

	data, _ := os.ReadFile(path)
	// 篡改:把某条判定从 REMUX 改成 IDENTICAL
	tampered := strings.Replace(string(data), "REMUX", "IDENTICAL", 1)
	if tampered == string(data) {
		t.Fatal("替换未生效")
	}
	os.WriteFile(path, []byte(tampered), 0o644)

	entries, _ := evidence.Load(path)
	if err := evidence.Verify(entries, pub); err == nil {
		t.Fatal("篡改后校验应失败")
	}
}

func TestChainDeletionDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chain.jsonl")
	pub, priv := genKeys(t)
	appendN(t, path, 3, priv)

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// 删除中间一条,破坏链式链接
	os.WriteFile(path, []byte(lines[0]+"\n"+lines[2]+"\n"), 0o644)

	entries, _ := evidence.Load(path)
	if err := evidence.Verify(entries, pub); err == nil {
		t.Fatal("删除条目后校验应失败")
	}
}

func TestChainBadSignature(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chain.jsonl")
	_, priv := genKeys(t)
	appendN(t, path, 1, priv)

	// 用另一把公钥校验,应失败
	otherPub, _ := genKeys(t)
	entries, _ := evidence.Load(path)
	if err := evidence.Verify(entries, otherPub); err == nil {
		t.Fatal("错误公钥应校验失败")
	}
}

func TestKeygenRoundtrip(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "k.priv")
	pubPath := filepath.Join(dir, "k.pub")
	if err := evidence.Keygen(privPath, pubPath); err != nil {
		t.Fatal(err)
	}
	priv, err := evidence.LoadKey(privPath)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := evidence.LoadKey(pubPath)
	if err != nil {
		t.Fatal(err)
	}
	chain := filepath.Join(dir, "c.jsonl")
	appendN(t, chain, 1, ed25519.PrivateKey(priv))
	entries, _ := evidence.Load(chain)
	if err := evidence.Verify(entries, ed25519.PublicKey(pub)); err != nil {
		t.Fatalf("落盘密钥往返校验失败: %v", err)
	}
}
