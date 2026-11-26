package digestx

import (
	"strings"
	"testing"
)

func TestSHA256KnownVector(t *testing.T) {
	got := SHA256Bytes([]byte("abc"))
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("sha256(abc)=%s, want %s", got, want)
	}
}

func TestEqualDigestRejectsPrefixAndMalformed(t *testing.T) {
	full := strings.Repeat("a", 64)
	if !EqualDigest(full, full) {
		t.Fatal("identical full digests must match")
	}
	prefix := full[:16]
	if EqualDigest(prefix, full) {
		t.Fatal("truncated/prefix digest must never compare equal")
	}
	if EqualDigest(strings.Repeat("z", 64), full) {
		t.Fatal("non-hex digest must not match")
	}
}

func TestChunkLocalization(t *testing.T) {
	data := []byte(strings.Repeat("A", 300))
	chunks := ChunkBytes(data, 128)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	modified := append([]byte(nil), data...)
	modified[150] = 'B' // inside chunk index 1: [128,256)
	chunks2 := ChunkBytes(modified, 128)
	for i := range chunks {
		equal := chunks[i].SHA256 == chunks2[i].SHA256
		if i == 1 && equal {
			t.Fatal("chunk 1 should differ after byte 150 changed")
		}
		if i != 1 && !equal {
			t.Fatalf("chunk %d should stay identical", i)
		}
	}
	if ChunkRoot(chunks) == ChunkRoot(chunks2) {
		t.Fatal("chunk root must commit to changed block hashes")
	}
}
