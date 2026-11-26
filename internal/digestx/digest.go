// Package digestx provides exact cryptographic digests and aligned block
// hashes used to localize partial tampering.
package digestx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// DefaultChunkSize is the aligned block size used when building an index.
// Real cases may use 64 KiB or larger; the reference case uses a small
// value so tamper localization is visible on tiny synthetic files.
const DefaultChunkSize = 64 << 10

// Chunk is one fixed-offset aligned block of a file.
type Chunk struct {
	Index  int    `json:"index"`
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// SHA256Bytes returns the lowercase hex SHA-256 of b.
func SHA256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SHA256Reader streams r through SHA-256 and returns lowercase hex.
func SHA256Reader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SHA256File hashes the file at path and returns digest and size.
func SHA256File(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	d, err := SHA256Reader(f)
	if err != nil {
		return "", 0, err
	}
	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	return d, st.Size(), nil
}

// ChunkBytes splits b into fixed-offset blocks of chunkSize and hashes each.
func ChunkBytes(b []byte, chunkSize int) []Chunk {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	chunks := make([]Chunk, 0, (len(b)+chunkSize-1)/chunkSize)
	for start := 0; start < len(b); start += chunkSize {
		end := start + chunkSize
		if end > len(b) {
			end = len(b)
		}
		part := b[start:end]
		chunks = append(chunks, Chunk{
			Index:  len(chunks),
			Start:  int64(start),
			End:    int64(end),
			Size:   end - start,
			SHA256: SHA256Bytes(part),
		})
	}
	return chunks
}

// ChunkRoot computes an order-binding Merkle-like digest over the block
// records. It commits to block index, size and digest under a domain
// separator so it cannot be confused with a plain file hash.
func ChunkRoot(chunks []Chunk) string {
	h := sha256.New()
	io.WriteString(h, "EVIDENCECHAIN/CHUNKROOT/v1\n")
	for _, c := range chunks {
		fmt.Fprintf(h, "%d:%d:%s\n", c.Index, c.Size, c.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// EqualDigest compares two hex digests case-insensitively. Only full-length
// (64 hex chars) values are considered; truncated/prefix strings never match.
func EqualDigest(a, b string) bool {
	if len(a) != 64 || len(b) != 64 {
		return false
	}
	ab, err1 := hex.DecodeString(a)
	bb, err2 := hex.DecodeString(b)
	if err1 != nil || err2 != nil {
		return false
	}
	if len(ab) != sha256.Size || len(bb) != sha256.Size {
		return false
	}
	return hex.EncodeToString(ab) == hex.EncodeToString(bb)
}
