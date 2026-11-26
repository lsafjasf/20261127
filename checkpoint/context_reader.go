package checkpoint

import (
	"context"
	"io"
)

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

// Read makes cancellation responsive even when the caller supplied a plain
// io.Reader. A permanently blocked underlying Read cannot be killed by Go, so
// its goroutine may linger until the source unblocks; it only owns the read
// buffer and never writes to the staging file after cancellation.
func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	type readResult struct {
		n   int
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		n, err := r.r.Read(p)
		done <- readResult{n: n, err: err}
	}()

	select {
	case result := <-done:
		return result.n, result.err
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}
