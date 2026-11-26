package batch

import (
	"sync"
)

// FlakyRenderer wraps a deterministic renderer and fails the first N
// attempts at a given (path, fragment) before succeeding. Because the inner
// renderer is pure, retried fragments produce byte-identical output.
type FlakyRenderer struct {
	inner    Renderer
	failLeft map[string]int
	mu       sync.Mutex
}

// NewFlakyRenderer creates a failure-injecting renderer. failSpec maps
// "path#index" keys to the number of consecutive failures before success.
func NewFlakyRenderer(inner Renderer, failSpec map[string]int) *FlakyRenderer {
	copied := make(map[string]int, len(failSpec))
	for key, count := range failSpec {
		copied[key] = count
	}
	return &FlakyRenderer{inner: inner, failLeft: copied}
}

// Render implements Renderer.
func (f *FlakyRenderer) Render(file FileSpec, fragmentIndex, length int) ([]byte, error) {
	key := fragmentKey(file.Path, fragmentIndex)
	f.mu.Lock()
	remaining := f.failLeft[key]
	if remaining > 0 {
		f.failLeft[key] = remaining - 1
	}
	f.mu.Unlock()

	if remaining > 0 {
		return nil, ErrInjected
	}
	return f.inner.Render(file, fragmentIndex, length)
}

func fragmentKey(path string, index int) string {
	return path + "#" + itoa(index)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
