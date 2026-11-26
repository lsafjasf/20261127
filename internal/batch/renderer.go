package batch

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

// Renderer produces deterministic output for one fragment.
//
// Implementations must be pure: identical (file, fragmentIndex, length)
// inputs must always return identical bytes. No timestamps, random values or
// environment-dependent data may enter the output; otherwise re-runs cannot
// keep hashes stable.
type Renderer interface {
	Render(file FileSpec, fragmentIndex, length int) ([]byte, error)
}

// DeterministicRenderer generates reproducible content from the file path,
// fragment index and a counter-driven byte stream.
type DeterministicRenderer struct{}

// ErrInjected models a transient rendering failure used for retry testing.
var ErrInjected = errors.New("injected rendering failure")

// NewDeterministicRenderer constructs the default renderer.
func NewDeterministicRenderer() DeterministicRenderer { return DeterministicRenderer{} }

// Render implements Renderer.
func (DeterministicRenderer) Render(file FileSpec, fragmentIndex, length int) ([]byte, error) {
	if length <= 0 {
		return nil, errors.New("fragment length must be positive")
	}
	buf := make([]byte, length)

	// Derive a per-file seed from the stable file path. The seed is mixed
	// with the fragment index so every fragment gets an independent stream.
	pathSum := sha256.Sum256([]byte(file.Path))
	seed := binary.BigEndian.Uint64(pathSum[:8]) ^
		uint64(fragmentIndex)*0x9E3779B97F4A7C15

	stream := splitmix64(seed)
	for offset := 0; offset < length; offset += 8 {
		word := stream()
		remaining := length - offset
		if remaining >= 8 {
			binary.BigEndian.PutUint64(buf[offset:offset+8], word)
		} else {
			var tmp [8]byte
			binary.BigEndian.PutUint64(tmp[:], word)
			copy(buf[offset:], tmp[:remaining])
		}
	}
	return buf, nil
}

// splitmix64 returns a deterministic pseudo-random word generator.
func splitmix64(seed uint64) func() uint64 {
	state := seed
	return func() uint64 {
		state += 0x9E3779B97F4A7C15
		z := state
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		return z ^ (z >> 31)
	}
}

// FragmentLength returns the deterministic byte length of one fragment.
func FragmentLength(file FileSpec, index int) int {
	if index == file.Fragments-1 {
		rendered := file.FragmentSize * (file.Fragments - 1)
		last := file.Size - rendered
		if last <= 0 {
			return file.FragmentSize
		}
		return last
	}
	return file.FragmentSize
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
